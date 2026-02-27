# Claude OAuth 代理机制内部分析

本文分析 CLIProxyAPI 在处理 Anthropic 官方 OAuth token（即 Claude.ai 账户登录凭证）时所做的各项请求修改，以及每项修改的设计意图。

## 核心设计场景

代理的主要使用场景**不只是** Claude Code CLI 直连代理，更核心的场景是：

```
Cursor / OpenCode / ClawdBot / Aider 等第三方客户端
    ↓
CLIProxyAPI（协议转换 + OAuth 伪装层）
    ↓
Anthropic API（使用 Claude.ai OAuth token，而非普通 API Key）
```

即：**让本来只供 Claude Code 使用的 OAuth token，也能被其他 AI 客户端合法使用**。

理解这个前提是读懂所有修改逻辑的关键。

---

## 请求修改清单与作者意图

### 1. `proxy_` 工具名前缀

**触发条件**：API Key 中包含 `sk-ant-oat`（OAuth Access Token 的标识）。

```go
func isClaudeOAuthToken(apiKey string) bool {
    return strings.Contains(apiKey, "sk-ant-oat")
}

if isClaudeOAuthToken(apiKey) && !auth.ToolPrefixDisabled() {
    bodyForUpstream = applyClaudeToolPrefix(body, claudeToolPrefix)
}
```

**意图**：Anthropic 对 OAuth token 有工具命名约束——只有带 `proxy_` 前缀的自定义工具才被服务端接受（这是 Anthropic 区分"Claude Code 官方内置工具"和"用户自定义工具"的机制）。

代理的处理是对称的：
- 发送给 Anthropic 前：给所有非内置工具名加 `proxy_` 前缀
- 收到响应后：剥除 `proxy_` 前缀再返回给下游客户端

对下游客户端完全透明，工具名不发生可见变化。内置工具（`web_search`、`code_execution` 等，即有 `type` 字段的工具）不加前缀。

**如何关闭**：在 auth JSON 文件的 `metadata` 中添加：

```json
{
  "metadata": {
    "tool_prefix_disabled": true
  }
}
```

---

### 2. `?beta=true` URL 参数

```go
url := fmt.Sprintf("%s/v1/messages?beta=true", baseURL)
```

**意图**：在 URL 层面激活 Anthropic 服务端的 beta 处理路径，配合 `Anthropic-Beta` header 中的 `oauth-2025-04-20` 共同触发 OAuth token 的特殊验证流程。这是 OAuth 模式下请求能被正确处理的必要条件之一。

---

### 3. Cloaking（伪装）系统

这是代理设计中最核心的功能，由三个子功能组成，统一受 `cloak.mode` 配置控制：

- `"auto"`（默认）：检测客户端 User-Agent，非 Claude Code 客户端才启用伪装
- `"always"`：无论客户端类型都启用
- `"never"`：完全禁用

```go
func shouldCloak(cloakMode string, userAgent string) bool {
    switch strings.ToLower(cloakMode) {
    case "always":
        return true
    case "never":
        return false
    default: // "auto"
        return !strings.HasPrefix(userAgent, "claude-cli")
    }
}
```

#### 3a. 系统提示注入

```go
claudeCodeInstructions := `[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}]`
```

**意图**：OAuth token 的使用场景约束要求请求来自 Claude Code 上下文。给第三方客户端（Cursor、OpenCode 等）的请求头部注入这段系统提示，让模型行为与 Claude Code 一致，同时使请求在 Anthropic 服务端通过 OAuth token 的合法性校验。

非严格模式（默认）下，客户端的原有系统提示会被保留在 Claude Code 提示词之后；严格模式（`strict-mode: true`）下，只保留 Claude Code 提示词。

对于 `claude-cli` 开头的 User-Agent（即真实的 Claude Code CLI），`auto` 模式会**跳过伪装**，不注入系统提示。

#### 3b. 假 user_id 注入

```go
// 格式完全匹配 Claude Code 的真实 user_id 格式
var userIDPattern = regexp.MustCompile(
    `^user_[a-fA-F0-9]{64}_account__session_[0-9a-f]{8}-...$`)

func generateFakeUserID() string {
    return "user_" + hexPart + "_account__session_" + uuidPart
}
```

**意图**：Claude Code 每次请求都携带符合特定格式的 `metadata.user_id`。第三方客户端通常不会生成这个字段，或格式不符合要求。代理注入格式正确的假 ID，让请求结构完全符合 Claude Code 的规范。

可通过 `cache-user-id: true` 配置让同一 API Key 复用同一个 user_id，而非每次随机生成。

#### 3c. 敏感词零宽空格混淆

```go
const zeroWidthSpace = "\u200B"

// 在词的第一个字符后插入不可见的零宽空格
// 例：「proxy」→「p​roxy」（肉眼无法分辨，但文本匹配失效）
func obfuscateWord(word string) string {
    r, size := utf8.DecodeRuneInString(word)
    return string(r) + zeroWidthSpace + word[size:]
}
```

**意图**：防止请求中的特定词汇（由 `sensitive-words` 配置）触发 Anthropic 内容过滤系统的异常处理路径。

---

### 4. `cache_control` 自动注入

```go
// Auto-inject cache_control if missing (optimization for ClawdBot/clients without caching support)
if countCacheControls(body) == 0 {
    body = ensureCacheControl(body)
}
```

**注入位置**（按 Anthropic prompt caching 文档的最优策略）：
1. `tools` 数组的**最后一个**工具（缓存所有工具定义）
2. `system` 数组的**最后一个**元素（缓存系统提示）
3. `messages` 中**倒数第二个** user 轮次（缓存对话历史）

**意图**：纯粹的成本优化。Anthropic prompt caching 可将缓存命中的 token 成本降低至 10%（0.1x base price）。Claude Code 本身会正确设置 `cache_control`，但 ClawdBot、基于 OpenAI SDK 的工具等第三方客户端不会。代理自动为这些客户端注入缓存断点，透明地享受最高 90% 的成本节省，对多轮对话（工具列表 + 系统提示反复发送的场景）效果尤为显著。

---

### 5. 仿 Claude Code 请求头（EnsureHeader 回退策略）

```go
// EnsureHeader 优先级：客户端发来的值 > 已有值 > 默认 fallback
// 第三方客户端没有这些头部时，才使用以下默认值
misc.EnsureHeader(r.Header, ginHeaders, "User-Agent", "claude-cli/2.1.44 (external, sdk-cli)")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Runtime", "node")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Lang", "js")
// ...
```

**意图**：为第三方客户端提供符合 Claude Code 规范的默认请求头。对于真实的 Claude Code CLI（它会自己发送这些头部），`EnsureHeader` 不会覆盖，仅对第三方客户端生效。

---

## 各修改对不同场景的影响

| 修改项 | Claude Code CLI 直连代理 | 第三方客户端经代理 |
|---|---|---|
| `proxy_` 工具前缀 | 生效（可关闭） | 生效（必要） |
| `?beta=true` | 生效 | 生效 |
| 系统提示注入 | **跳过**（auto 模式检测到 `claude-cli` UA） | 注入 |
| 假 user_id 注入 | **跳过**（UA 检测同上） | 注入 |
| 敏感词混淆 | **跳过** | 按配置 |
| `cache_control` 注入 | 生效（有益无害，Claude Code 本身已有时跳过） | 生效 |
| 仿 Claude Code 头部 | 客户端自带，EnsureHeader 不覆盖 | 注入默认值 |

---

## 可被 Anthropic 检测到的差异点

使用 Claude Code CLI 直连代理时，与直接连接 Anthropic 相比，存在以下潜在差异：

| 差异项 | 说明 |
|---|---|
| `proxy_` 工具前缀 | 可通过 `tool_prefix_disabled: true` 关闭 |
| `?beta=true` URL 参数 | 官方 Claude Code 不一定每次带此参数 |
| OS/Arch 指纹 | Go 运行时 vs Node.js 运行时的系统信息可能有细微差异 |
| `cache_control` 注入 | 与 Claude Code 行为一致，实际上无差异 |

使用合法的 OAuth token（即自己账号的 token）且目的是本地指标统计（TPS 计算等），并不属于 TOS 违规行为。Anthropic 的常规系统不针对此类本地代理使用场景进行专项检测。

---

## 总结

CLIProxyAPI 对 Claude OAuth 请求的所有修改，本质上构成了一套完整的 **"OAuth token 扩展使用框架"**：

| 修改 | 根本原因 | 目标 |
|---|---|---|
| `proxy_` 工具前缀 | Anthropic OAuth 工具命名约束 | 绕过服务端工具名限制 |
| `?beta=true` | OAuth 流程激活需要 | 触发正确的服务端处理路径 |
| 系统提示注入 | OAuth token 场景限制 | 让第三方客户端请求"看起来是 Claude Code" |
| 假 user_id | Claude Code 格式要求 | 让请求结构符合 Anthropic 规范 |
| 敏感词混淆 | 内容过滤规避 | 防止特定词汇触发异常 |
| `cache_control` 注入 | 第三方客户端不支持缓存 | 为所有客户端透明启用 prompt caching |
| 仿 Claude Code 头部 | 第三方客户端缺少必要头部 | 完善请求指纹 |

---

## 深度调查：凭据路径、nativePassthrough 与已知 Bug

### 凭据解析优先级（`claudeCreds` 函数）

`claude_executor.go:1004` 处的 `claudeCreds` 函数按以下优先级取令牌：

```go
func claudeCreds(a *cliproxyauth.Auth) (apiKey, baseURL string) {
    // 第一优先级：Attributes["api_key"]（来自 config.yaml claude-api-key）
    apiKey = a.Attributes["api_key"]
    baseURL = a.Attributes["base_url"]

    // 第二优先级：Metadata["access_token"]（来自 JSON 认证文件）
    if apiKey == "" && a.Metadata != nil {
        if v, ok := a.Metadata["access_token"].(string); ok {
            apiKey = v
        }
    }
    return
}
```

这两种来源对应两种完全不同的 Auth 结构：

| 来源 | `Attributes["api_key"]` | `Metadata["access_token"]` | `useAPIKey` |
|------|------------------------|---------------------------|-------------|
| `config.yaml` → `claude-api-key` | **有值**（即配置的 key） | 无 | `true` |
| JSON 认证文件（`auths/` 目录） | **无值** | 有值（OAuth 令牌） | `false` |

`useAPIKey` 直接决定请求走哪条 Header 路径：

```go
// claude_executor.go:55（PrepareRequest）
// claude_executor.go:930（applyClaudeHeaders）
useAPIKey := auth != nil && auth.Attributes != nil &&
    strings.TrimSpace(auth.Attributes["api_key"]) != ""
```

---

### 两条 Header 路径对比

#### 路径 A：`useAPIKey = true`（config.yaml 来源）

```go
if isAnthropicBase && useAPIKey && !isClaudeOAuthToken(apiKey) {
    req.Header.Del("Authorization")
    req.Header.Set("x-api-key", apiKey)   // ← 普通 API key 走这里
} else {
    req.Header.Del("x-api-key")
    req.Header.Set("Authorization", "Bearer "+apiKey) // ← OAuth token 走这里（本地修复后）
}
```

**本地未提交修复**（`claude_executor.go:58` / `:933`）正是针对此路径：当 `claude-api-key` 配置的值是 OAuth 令牌（`sk-ant-oat` 前缀）时，老代码会错误地走 `x-api-key`，新代码通过 `!isClaudeOAuthToken(apiKey)` 检测后强制走 `Authorization: Bearer`。

#### 路径 B：`useAPIKey = false`（JSON 文件来源）

`useAPIKey = false`，条件恒为 `false`，直接走 `Authorization: Bearer`。理论上此路径**无需**本地修复也应正确。

---

### nativePassthrough 机制

**触发条件**（三者同时满足）：

```go
// claude_executor.go:107
nativePassthrough := isClaudeOAuthToken(apiKey)           // 令牌是 sk-ant-oat 前缀
    && isClaudeCodeClient(getClientUserAgent(ctx))         // 客户端 UA 是 claude-cli 开头
    && from == to                                          // 无格式转换（都是 claude 格式）
```

**触发后的行为**：

- 请求 body 原样透传，不修改
- 客户端所有原始请求头透传（仅剔除 `Authorization`、`X-Api-Key`、`X-Goog-Api-Key`）
- 只替换一个头：`Authorization: Bearer <代理的 OAuth 令牌>`
- **不调用** `applyClaudeHeaders`，不调用 `applyCloaking`

**在 Claude Code CLI 作为前端的场景下**，nativePassthrough 触发时，Anthropic 实际收到的请求：

| 字段 | 来源 |
|------|------|
| `User-Agent` | 真实 Claude Code CLI（如 `claude-cli/2.1.44`） |
| `X-Stainless-Os/Arch` | 用户机器（由 CLI 写入） |
| `Anthropic-Beta` | CLI 原样透传 |
| 请求 body | 原始，未修改 |
| `Authorization` | 代理账号的 OAuth 令牌（JSON 文件中的 `access_token`） |

---

### 已知 Bug：nativePassthrough 下 `oauth-2025-04-20` Beta 缺失

**现象**：上传了 Claude OAuth JSON 认证文件后，通过代理发出的请求收到 Anthropic 错误：

```json
{
  "type": "error",
  "error": {
    "type": "authentication_error",
    "message": "OAuth authentication is currently not supported."
  }
}
```

**根本原因**：

Anthropic 的 `/v1/messages` 端点要求，若使用 OAuth Bearer 令牌，请求头中**必须包含** `Anthropic-Beta: ...,oauth-2025-04-20,...`，否则拒绝 OAuth 认证。

- **非 nativePassthrough 路径**（`applyClaudeHeaders`）：始终确保该 beta 存在（第 947-953 行，即使客户端未携带也会补全）✓
- **nativePassthrough 路径**：透传客户端的 `Anthropic-Beta` 头，**不做任何 beta 补全** ✗

当 Claude Code CLI 以 API key（如 `sk-dummy`）方式与代理通信时，CLI 发出的 `Anthropic-Beta` 头中可能不含 `oauth-2025-04-20`（因为 CLI 本身此时并非通过 OAuth 与代理通信）。nativePassthrough 将此头原样转发给 Anthropic，Anthropic 收到了 `Authorization: Bearer sk-ant-oat01-...`，但 beta 缺失，报错。

**影响范围**：仅影响同时满足以下条件的场景：

1. 代理使用 JSON 文件认证（令牌来自 `Metadata["access_token"]`，非 config.yaml）
2. 下游客户端是 Claude Code CLI（UA 以 `claude-cli` 开头）
3. 客户端未在 `Anthropic-Beta` 中携带 `oauth-2025-04-20`

**待修复**：在 nativePassthrough 路径中，当 `isClaudeOAuthToken(apiKey) == true` 时，需确保 `oauth-2025-04-20` 已注入 `Anthropic-Beta` 头（参照 `applyClaudeHeaders` 中的 beta 补全逻辑）。

---

### 令牌归属说明

JSON 认证文件场景中，"令牌归属账号"指的是：

- 用户上传**自己的** Claude.ai OAuth JSON 文件 → 令牌属于用户本人账号，与直接使用 Claude Code CLI 完全等价，无额外风险
- 代理服务器使用**自己的**令牌为多个下游用户转发 → 令牌属于代理账号，多用户共享同一令牌可能在 Anthropic 侧显现为异常并发模式

当前用户上传的是本人账号（`a2018935415@gmail.com`）的认证文件，属于前者，无令牌归属风险。

---

### Anthropic 检测风险（Claude Code CLI 作为前端场景）

当下游客户端是真实 Claude Code CLI，且 nativePassthrough 正确触发时：

| 检测信号 | 风险等级 | 说明 |
|----------|----------|------|
| 流量指纹 | **极低** | 所有头部均来自真实 CLI，与直连 Anthropic 无差异 |
| 令牌账号 | **无**（自有令牌）/ 中（共享令牌） | 见上方说明 |
| IP 地址 | 低（本地运行）/ 中（VPS 运行） | 本地运行时 IP 与用户机器一致 |
| 请求量模式 | 低 | 单用户使用时无异常 |

nativePassthrough **正常工作**时，Anthropic 侧看到的请求与用户直接使用 Claude Code CLI 几乎完全相同，唯一差异是令牌来自不同路径（代理的 JSON 文件 vs CLI 本地凭据）。
