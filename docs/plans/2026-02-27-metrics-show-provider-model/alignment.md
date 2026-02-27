# Alignment: metrics_summary 两个修复 + Claude token 字段完整性

## Requirements

### Bug 1：缺少 provider 和 model

`metrics_summary` 日志输出（写入 stderr）当前仅显示三个字段：

```
metrics_summary request_count=29
metrics_summary time_window=10m
metrics_summary tps_avg=26.947
```

缺少本次请求所使用的 `provider` 和 `model`，无法从日志中判断是哪个提供商/模型在响应。

**成功标准：**
- 每次请求结束后，`metrics_summary` 包含 `provider` 和 `model` 字段（多行格式）：
  ```
  metrics_summary provider=claude
  metrics_summary model=claude-sonnet-4-6
  metrics_summary request_count=29
  metrics_summary time_window=10m
  metrics_summary tps_avg=26.947
  ```
- 若 provider 或 model 为空字符串，仍输出空值行
- 不影响 management API、SQLite 持久化等

---

### Bug 2：部分请求显示 `request_count=0` / `tps_avg=--`

某些请求（尤其是 `POST /v1/messages?beta=true`，状态 200，耗时 ~2s）显示：

```
metrics_summary request_count=0
metrics_summary time_window=10m
metrics_summary tps_avg=--
```

**成功标准：**
- 修复后，所有完成的 nativePassthrough 请求都能正确累积 E2E 窗口统计
- `request_count` 和 `tps_avg` 显示有意义的数值

---

### 附加修复：Claude token 字段提取不完整（正确性 bug）

`parseClaudeUsage` / `parseClaudeStreamUsage` 当前遗漏了 Claude API 实际返回的以下字段：

**来自真实 OAuth 模式抓包（`docs/claude-cli-2.1.62-request-oauth登录模式.txt`）：**

```json
// message_start 事件（usage 嵌套在 message 内，当前不被 parseClaudeStreamUsage 解析）
{
  "type": "message_start",
  "message": {
    "usage": {
      "input_tokens": 3,
      "cache_creation_input_tokens": 0,
      "cache_read_input_tokens": 32202,
      "cache_creation": {          // ← 当前未解析
        "ephemeral_5m_input_tokens": 0,
        "ephemeral_1h_input_tokens": 0
      },
      "output_tokens": 1,
      "service_tier": "standard",
      "inference_geo": "not_available"
    }
  }
}

// message_delta 事件（usage 在顶层，当前已正确解析）
{
  "type": "message_delta",
  "usage": {
    "input_tokens": 3,
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 32202,
    "output_tokens": 20
  }
}
```

**当前遗漏字段：**
- `cache_creation.ephemeral_5m_input_tokens`（短效缓存创建 token）
- `cache_creation.ephemeral_1h_input_tokens`（长效缓存创建 token）
- `message_start` 中嵌套的 `message.usage`（当前完全跳过，不解析）
- `thinking_tokens`（未在真实抓包中出现，可能是推测字段，待确认）

**注意：** `message_delta` 中的 `output_tokens` 是最终值，已被正确解析。`message_start` 中的 `output_tokens: 1` 是初始占位值，不需要使用。`CachedTokens` 当前用 `cache_read_input_tokens` 填充，`cache_creation_input_tokens` 作为备用——这个逻辑本身是对的，但忽略了 `cache_creation.ephemeral_*` 字段。

**成功标准：**
- `cache_creation.ephemeral_5m_input_tokens` 和 `ephemeral_1h_input_tokens` 被加入 `CachedTokens`（或单独记录）
- `ReasoningTokens` 字段保留，若 Claude 未来返回 `thinking_tokens` 则可直接启用

---

## Architecture

### Bug 1 根本原因（已确认，简单）

`RequestStateSnapshot`（`internal/metricsruntime/request_state.go`）已有 `Provider` 和 `Model` 字段，但 `PrintSummary()`（`internal/metricsruntime/display.go:54-64`）未读取它们。

**修改范围：** 仅 `display.go` 的 `PrintSummary()` 函数，添加两行 `fmt.Fprintf`。

---

### Bug 2 根本原因（已确认路径，仍需确认具体触发条件）

**E2E 条件链（任一失败 → count=0）：**
```
usage_plugin.go:261: if state != nil && hasKey && !isCanceled && outputTokens > 0 && durationMs > 0
```

**已排除的嫌疑：**
- 流式响应中 `message_delta` 的 `output_tokens` 在真实抓包中为 20（非零）
- `parseClaudeStreamUsage` 对 `message_delta` 的解析逻辑正确
- `message_start` 中嵌套的 usage 未解析，但 `output_tokens=1` 的初始值无需使用

**剩余可能原因：**

| 原因 | 可能性 | 证据 |
|------|--------|------|
| `outputTokens=0`：某些请求的 `message_delta` 中 `output_tokens` 确实为 0（纯工具调用、超短响应等） | 高 | 与 ~2s 短响应时间吻合 |
| `hasKey=false`：nativePassthrough 路径中 provider/model 未在 state 中设置 | 中 | Bug 1 修复后可诊断 |
| `outputTokens=0`：`publishWithOutcome` 的空 usage 保护逻辑触发（InputTokens=0 且 OutputTokens=0） | 中 | `ensurePublished` 发布空 Detail |

**Bug 1 修复是诊断 Bug 2 的前提**：修复后如果 provider/model 行显示为空，说明 `hasKey=false`；如果非空，说明 `outputTokens=0` 是根因。

**修改范围：**

| 文件 | 位置 | 说明 |
|------|------|------|
| `internal/metricsruntime/display.go` | `PrintSummary()` | Bug 1：添加 provider/model |
| `internal/metricsruntime/usage_plugin.go` | E2E 条件（第 261 行） | Bug 2：用 `totalTokens`（含 reasoning）替代单独的 `outputTokens` 条件，确保有 input tokens 时也触发 E2E |

**Bug 2 的修复逻辑：** 当 `outputTokens=0` 但 `inputTokens>0` 时（如纯思考/纯工具调用请求），也应纳入 E2E 统计。TPS 计算改用 `totalTokens - inputTokens`（即生成 token 数）：

```go
// 修复前
if ... && outputTokens > 0 && ...

// 修复后：生成 token 数 = output + reasoning
generatedTokens := outputTokens + reasoningTokens
if generatedTokens == 0 {
    generatedTokens = max(0, totalTokens - inputTokens)
}
if ... && generatedTokens > 0 && ...
```

---

### 附加修复：Claude token 字段提取

`parseClaudeUsage` 和 `parseClaudeStreamUsage`（`internal/runtime/executor/usage_helpers.go:271-308`）需补充：

```go
// 当前：
detail := usage.Detail{
    InputTokens:  usageNode.Get("input_tokens").Int(),
    OutputTokens: usageNode.Get("output_tokens").Int(),
    CachedTokens: usageNode.Get("cache_read_input_tokens").Int(),
}
if detail.CachedTokens == 0 {
    detail.CachedTokens = usageNode.Get("cache_creation_input_tokens").Int()
}
detail.TotalTokens = detail.InputTokens + detail.OutputTokens

// 修复后（新增 ephemeral cache tokens 和 ReasoningTokens）：
detail := usage.Detail{
    InputTokens:  usageNode.Get("input_tokens").Int(),
    OutputTokens: usageNode.Get("output_tokens").Int(),
    CachedTokens: usageNode.Get("cache_read_input_tokens").Int(),
}
// ephemeral cache tokens 计入 CachedTokens
ephemeral5m := usageNode.Get("cache_creation.ephemeral_5m_input_tokens").Int()
ephemeral1h := usageNode.Get("cache_creation.ephemeral_1h_input_tokens").Int()
detail.CachedTokens += ephemeral5m + ephemeral1h
if detail.CachedTokens == 0 {
    detail.CachedTokens = usageNode.Get("cache_creation_input_tokens").Int()
}
// thinking tokens（按 Claude API 约定，output_tokens 已包含；此为保留字段）
detail.ReasoningTokens = usageNode.Get("thinking_tokens").Int()
detail.TotalTokens = detail.InputTokens + detail.OutputTokens + detail.ReasoningTokens
```

---

## Technology Decisions

- **Bug 1 输出格式**：多行 `key=value`，`provider` 和 `model` 作为前两行
- **Bug 1 空值**：直接输出 `provider=`，不做条件判断
- **Bug 2 E2E token 基准**：使用 `(output + reasoning)` 或 `(total - input)` 作为生成 token 数；两种计算等价，取前者避免依赖 `TotalTokens` 是否被正确填充
- **ephemeral cache tokens**：加入 `CachedTokens`（而非新建字段），与现有 `cache_read` / `cache_creation` 语义一致
- **thinking_tokens 字段**：按 `ReasoningTokens` 存储，当 API 未返回时为 0，天然向后兼容
- **TotalTokens 计算**：更新为 `InputTokens + OutputTokens + ReasoningTokens`，与 Gemini/OpenAI 解析器保持一致
- **不修改 E2E 窗口存储结构**：`RequestWindowStatsE2E` 无需新增字段
