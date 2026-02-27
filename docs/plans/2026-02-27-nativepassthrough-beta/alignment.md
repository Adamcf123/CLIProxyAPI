# Alignment: nativePassthrough Beta Header 策略

## Requirements

### 问题陈述

nativePassthrough 路径（触发条件：OAuth token + `claude-cli` UA + 无格式转换）目前完全透传客户端的 `Anthropic-Beta` header，不做任何修改。当下游 CLI 以 API key 模式连接代理时，其 `Anthropic-Beta` 不含 `oauth-2025-04-20`，导致上游 Anthropic 返回：

```json
{"type": "error", "error": {"type": "authentication_error", "message": "OAuth authentication is currently not supported."}}
```

### 核心需求

1. **必须**：nativePassthrough 路径中，当上游 token 是 OAuth token 时，确保 `Anthropic-Beta` 包含 `oauth-2025-04-20`
2. **策略选择**：确定 append（追加）还是 replace（替换）策略
3. **精确性**：最终 beta 列表尽可能接近 CLI 在 OAuth 原生模式下的真实发送值
4. **可维护性**：随 CLI 版本迭代，beta 集合的更新机制要可操作

### 成功标准

- nativePassthrough 路径不再因 beta 缺失而认证失败
- 结果的 beta 列表与 CLI 原生 OAuth 模式的差异最小
- 新版本 CLI 更新 beta 时，有明确的更新路径

---

## Architecture

### 当前 applyClaudeHeaders 的混合策略（非 nativePassthrough 路径）

```
internal/runtime/executor/claude_executor.go:947-972
```

策略为 **Replace + Append**：
- 客户端发送了 beta → **Replace**（用客户端值替换硬编码默认值）
- 确保 `oauth-2025-04-20` → **Append**（缺失时追加）
- 确保 `prompt-caching-2024-07-31` → **Append**（缺失时追加）
- 合并请求体 `betas` 字段 → **Append**（去重）

### nativePassthrough 路径的当前状态

```
claude_executor.go:170-186（Execute）
claude_executor.go:354-369（ExecuteStream）
claude_executor.go:526-541（CountTokens）
```

当前行为：客户端 headers 完全透传，**无任何 beta 处理**。

### Append vs Replace 分析

#### CLI 在 API-key 模式发送的 beta（连接代理时）
```
claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28
（无 oauth-2025-04-20）
```
**实测验证**：2026-02-27 mitmdump 反向代理抓包（CLI 2.1.62，API-key 模式连接代理 53355），确认上述值准确。

#### CLI 在 OAuth 原生模式发送的 beta（直连 Anthropic 时，2.1.62 抓包）
```
claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28
```

#### 两者的差异

| beta | API-key 模式 | OAuth 原生模式 |
|------|------------|--------------|
| `claude-code-20250219` | ✓ | ✓ |
| `oauth-2025-04-20` | ✗ | ✓ |
| `interleaved-thinking-2025-05-14` | ✓ | ✓ |
| `prompt-caching-scope-2026-01-05` | ✓ | ✓ |
| `effort-2025-11-24` | ✓ | ✓ |
| `adaptive-thinking-2026-01-28` | ✓ | ✓ |

**关键发现**：两种模式的 beta 差异只有 `oauth-2025-04-20` 一项。
→ **Append 策略在当前 CLI 版本下即可达到与 OAuth 原生模式完全相同的结果。**

**已知风险**：未来 CLI 若在 OAuth 模式下引入专属 beta（API-key 模式不发送的），append 会遗漏。缓解手段是持续抓包对比两种模式的 beta 差异；当前版本无需验证。

#### 如果 Anthropic 加强 beta 校验

| 校验方式 | Append | Replace（硬编码 OAuth 集） |
|---------|--------|------------------------|
| presence check（当前） | ✓ | ✓ |
| OAuth token 只允许特定 beta 白名单 | ✓（CLI 发的都是合法 beta） | ✓ |
| OAuth + CLI UA 必须精确等于 OAuth 集 | ✗（多了或少了） | ✓（如果硬编码准确） |

**结论**：Append 更贴合实际，Replace 在极端校验下更安全但依赖硬编码准确性。

---

### "精确 Append"的含义

"精确"指：在 nativePassthrough 路径中，**只追加 `oauth-2025-04-20`**（如果缺失），其余 beta 完全来自 CLI 的原始请求。

这与 `applyClaudeHeaders` 中的 oauth beta 确保逻辑完全一致：

```go
// applyClaudeHeaders 中的模式（claude_executor.go:950-952）
if !strings.Contains(val, "oauth") {
    baseBetas += ",oauth-2025-04-20"
}
```

nativePassthrough 中应复用相同逻辑，仅在 `isClaudeOAuthToken(apiKey) == true` 时触发。

---

### Beta 更新机制

#### 当前问题

`applyClaudeHeaders` 中的硬编码默认 beta（`claude_executor.go:947`）已经过时：

| beta | 硬编码值 | 2.1.62 实测 OAuth 模式 |
|------|---------|----------------------|
| `claude-code-20250219` | ✓ | ✓ |
| `oauth-2025-04-20` | ✓ | ✓ |
| `interleaved-thinking-2025-05-14` | ✓ | ✓ |
| `fine-grained-tool-streaming-2025-05-14` | ✓（多余） | ✗ |
| `prompt-caching-2024-07-31` | ✓（过时） | ✗ |
| `prompt-caching-scope-2026-01-05` | ✗（缺失） | ✓ |
| `effort-2025-11-24` | ✗（缺失） | ✓ |
| `adaptive-thinking-2026-01-28` | ✗（缺失） | ✓ |

**这是一个独立的副问题**：硬编码默认值更新问题适用于非 nativePassthrough 路径（第三方客户端未发送 Anthropic-Beta 时）。

#### 更新策略选项

**方案 A：手动更新常量（当前方案）**
在每次 CLI 版本变更后，通过抓包更新 `claude_executor.go:947` 的硬编码字符串。
- 优点：零复杂度
- 缺点：依赖人工感知版本变化

**方案 C：运行时自学习（未来理想方案，当前不可行）**
代理检测到真实 CLI OAuth 请求时，记录其 `Anthropic-Beta` 作为参考集。
- 优点：全自动，永远准确
- 缺点：**当前 CLI 只能通过 API key 连接代理，无法直接以 OAuth 模式发请求给代理**，该方案没有输入来源；待架构支持 CLI 以 OAuth 模式连接代理后方可启用

---

## Technology Decisions

### 决定 1：nativePassthrough 路径采用 Append 策略

**选定**：Append（仅补全缺失的 `oauth-2025-04-20`）

**理由**：
- 当前 CLI 版本两种模式的 beta 差异只有一项 `oauth-2025-04-20`，append 即可精确还原 OAuth 原生模式
- 与 `applyClaudeHeaders` 中现有的 oauth beta 确保逻辑一致，无需引入新模式
- Replace 需要维护硬编码 beta 集且已证明会过时，收益不足以抵消维护成本
- Anthropic 加强校验的风险低：即使校验为白名单，CLI API-key 模式发送的 beta 均为合法 beta

**实现位置**：三处 nativePassthrough 的 header 复制循环之后，`httpReq.Header.Set("Authorization", "Bearer "+apiKey)` 之前，复用 `applyClaudeHeaders` 中相同的单行 oauth beta 检测与 append 逻辑。

### 决定 2：applyClaudeHeaders 中硬编码默认值同步更新

**选定**：将 `claude_executor.go:947` 的硬编码默认 beta 完全替换为 2.1.62 抓包实测值，不保留任何抓包中未出现的 beta。

新值：
```
claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28
```

移除：`fine-grained-tool-streaming-2025-05-14`、`prompt-caching-2024-07-31`

### 决定 3：Beta 更新机制

**选定**：方案 A（手动更新），不做配置化

**依据**：
- CLI 目前只能以 API key 模式连接代理，方案 C（自学习）的输入来源不存在
- 配置化（方案 B）对用户没有实际价值——用户不知道要改什么 beta
- 手动更新基于抓包，流程清晰：捕获 CLI 新版本 OAuth 直连流量 → 提取 `Anthropic-Beta` → 更新 `claude_executor.go:947`

**参考记录**：在 `docs/claude-beta-reference.md` 中记录已验证 CLI 版本与对应 OAuth beta 字符串，作为更新历史。
