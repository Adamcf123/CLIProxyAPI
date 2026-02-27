# Alignment: Claude CLI 完全透传

## 评价：用户的判断是正确的

通过 mitmproxy 对 claude-cli/2.1.62 的抓包，证实 claude-cli 发出的请求已经是自洽的——
它自带所有必要的头部、正确的 beta 列表、cache_control 断点和真实 user_id。
代理当前对 cli 请求的所有"修改"，要么多余，要么引入偏差。

**实测对比（抓包数据来自 docs/request.txt）：**

| 项目 | cli 直连 Anthropic | 代理当前行为 | 差异 |
|---|---|---|---|
| URL | `/v1/messages?beta=true` | `/v1/messages?beta=true` | 无（一致） |
| `Anthropic-Beta` | `...prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28` | 客户端值 + 追加 `prompt-caching-2024-07-31` | **多追加过时 beta** |
| `X-Stainless-Helper-Method` | 不发送 | 通过 EnsureHeader 补 `stream` | **多发一个头** |
| `cache_control` | cli 自带，计数 > 0 | 计数 > 0 时跳过注入 | 无（已正确跳过） |
| `proxy_` 工具前缀 | 不加前缀直接发 | 加前缀发送、收到后剥除 | 对称但多余；有 bug 风险 |
| `metadata.user_id` | 真实格式含 account UUID | 伪造，格式缺少 account UUID 段 | cli 自带真实值，proxy 不会注入（auto 模式跳过 cloaking），格式 bug 只影响第三方客户端 |

**结论：对 cli 客户端，代理唯一应做的事是换 Authorization token，其余一律透传。**

---

## Requirements

### 目标

当上游客户端是 Claude Code CLI（User-Agent 以 `claude-cli` 开头）时，代理进入
**native passthrough 模式**：仅替换 Authorization token，不对请求体和其他头部做任何修改。

### 成功标准

1. cli 发出的请求体（包含 tools 定义、cache_control、metadata）原样转发，不增删字段
2. cli 发出的 `Anthropic-Beta` 头原样转发，不追加任何额外 beta
3. 代理不向请求添加 `X-Stainless-Helper-Method` 或其他 cli 未发送的头部
4. 响应体原样回传，不做 proxy_ 前缀剥除
5. 指标采集（TPS/TTFT/TPOT）继续正常工作
6. 第三方客户端（非 `claude-cli` UA）的现有行为不受影响

### 不在范围内

- 修改第三方客户端路径的任何逻辑
- 修改 cloaking 配置语义（tool_prefix_disabled、cloak.mode 等）
- 修复 user_id 格式 bug（独立问题，不属于本次范围）

---

## Architecture

### 检测点（已有）

`cloak_utils.go:44` 已有 `isClaudeCodeClient(userAgent string) bool`：

```go
func isClaudeCodeClient(userAgent string) bool {
    return strings.HasPrefix(userAgent, "claude-cli")
}
```

`claude_executor.go:1142` 已有 `getClientUserAgent(ctx) string`。

`shouldCloak` 已在 `cloak_utils.go:32` 用相同逻辑为 cloaking 子系统实现了跳过。
native passthrough 模式是将这个跳过逻辑扩展到整个修改管线。

### 修改管线中需要跳过的节点

`claude_executor.go` Execute 方法（非流式）：

| 行号 | 当前操作 | native passthrough 时 |
|---|---|---|
| 127 | `applyCloaking(...)` | 已由 shouldCloak auto 模式跳过 |
| 142-143 | `ensureCacheControl(body)` | 跳过（cli 已自带，但逻辑上应显式跳过） |
| 151-152 | `applyClaudeToolPrefix(...)` | 跳过 |
| 160 | `applyClaudeHeaders(...)` | 跳过；仅在调用处手动设 `Authorization: Bearer <token>` |
| 225-226 | `stripClaudeToolPrefixFromResponse(...)` | 跳过 |

ExecuteStream 方法（流式）有镜像逻辑，同步处理。

### 引入一个布尔短路变量

在 Execute / ExecuteStream 入口处计算 `nativePassthrough bool`：

```
nativePassthrough = isClaudeOAuthToken(apiKey) && isClaudeCodeClient(clientUA)
```

后续各修改节点用 `if !nativePassthrough` 包裹。

### 头部处理分叉

native passthrough 时，**在调用处直接跳过 `applyClaudeHeaders`**，仅手动设置
`Authorization: Bearer <token>`，其余头部由 cli 客户端自己发送、自然透传。
对 `applyClaudeHeaders` 函数本身零侵入。

---

## Technology Decisions

### 检测方式：User-Agent 前缀（沿用现有）

`isClaudeCodeClient` 已存在且被 shouldCloak 使用，行为与 auto 模式一致。
不引入新的配置开关——native passthrough 对 cli 客户端是无条件行为。

### 触发条件：双重检测

`isClaudeOAuthToken(apiKey) && isClaudeCodeClient(clientUA)`

只有 OAuth token + cli UA 同时满足才进入 native passthrough。
非 OAuth token（普通 API key）走原有逻辑，不受影响。

### 变量命名

使用 `nativePassthrough`，语义明确：这是原生 cli 请求，不需要代理修改。

### 测试策略

现有 `claude_executor_test.go` 中有对 cloaking 的测试。
需增加：cli UA + OAuth token 场景下，请求体和 Beta 头不被修改的断言。
