# Task 001 (Green): 实现 Native Passthrough

**depends-on:** task-001-native-passthrough-test

## BDD Scenario

```gherkin
Scenario: A — 请求体原样转发，无 proxy_ 工具前缀
  Given 客户端使用 OAuth token（api_key 含 "sk-ant-oat"）
  And 客户端 User-Agent 为 "claude-cli/2.1.62 (external, sdk-cli)"
  And 请求体包含自定义工具 "Read" 和 "Write"
  When Execute() 被调用
  Then 上游服务器收到的请求体中工具名为 "Read" 和 "Write"（未被加 "proxy_" 前缀）

Scenario: B — Anthropic-Beta 头原样转发，不追加额外 beta
  Given 客户端使用 OAuth token
  And 客户端 User-Agent 为 "claude-cli/2.1.62 (external, sdk-cli)"
  And 客户端发送 Anthropic-Beta: "claude-code-20250219,oauth-2025-04-20,prompt-caching-scope-2026-01-05"
  When Execute() 被调用
  Then 上游服务器收到的 anthropic-beta 头值与客户端发送的值完全一致（不含额外追加的 beta）

Scenario: C — 不注入 X-Stainless-Helper-Method 头
  Given 客户端使用 OAuth token
  And 客户端 User-Agent 为 "claude-cli/2.1.62 (external, sdk-cli)"
  And 客户端请求不含 X-Stainless-Helper-Method 头
  When Execute() 被调用
  Then 上游服务器收到的请求不含 X-Stainless-Helper-Method 头

Scenario: D — 流式响应不做 proxy_ 前缀剥除
  Given 客户端使用 OAuth token
  And 客户端 User-Agent 为 "claude-cli/2.1.62 (external, sdk-cli)"
  And 上游返回 SSE 流，其中含工具名 "proxy_Read"
  When ExecuteStream() 被调用
  Then 下游收到的 SSE 行中工具名仍为 "proxy_Read"（未被剥除前缀变成 "Read"）

Scenario: E — 非 cli UA 时 proxy_ 前缀照常应用（回归保护）
  Given 客户端使用 OAuth token
  And 客户端 User-Agent 为 "cursor/1.0"（非 claude-cli）
  And 请求体包含自定义工具 "Read"
  When Execute() 被调用
  Then 上游服务器收到的请求体中工具名为 "proxy_Read"（proxy_ 前缀正常应用）
```

## 要修改的文件

- `internal/runtime/executor/claude_executor.go`

## 步骤描述

### 步骤 1：在 Execute() 中计算 nativePassthrough 标志

在 `Execute()` 方法入口，确定 `apiKey` 之后、第一个 `applyCloaking` 调用之前，用 `getClientUserAgent(ctx)` 获取客户端 UA，计算布尔变量：

```
nativePassthrough = isClaudeOAuthToken(apiKey) && isClaudeCodeClient(clientUA)
```

### 步骤 2：用 nativePassthrough 跳过 ensureCacheControl（Execute）

用 `if !nativePassthrough` 包裹 line 142-143 的 `ensureCacheControl` 调用块，使 native passthrough 时不注入 `cache_control`。

### 步骤 3：修改 Execute() 中 proxy_ 前缀的应用条件

将 line 151-152 的条件从：

```
if isClaudeOAuthToken(apiKey) && !auth.ToolPrefixDisabled()
```

改为：

```
if isClaudeOAuthToken(apiKey) && !auth.ToolPrefixDisabled() && !nativePassthrough
```

当 `nativePassthrough` 为 true 时，`bodyForUpstream` 直接等于 `body`，工具名不加 `proxy_` 前缀。

### 步骤 4：修改 Execute() 中 applyClaudeHeaders 的调用逻辑

将 line 160 的 `applyClaudeHeaders(...)` 调用改为分支逻辑：

- 当 `!nativePassthrough`：保持原有的 `applyClaudeHeaders(...)` 调用不变。
- 当 `nativePassthrough`：
  1. 从 gin context 的 `ginHeaders`（客户端原始请求头）复制所有头部到 `httpReq.Header`。
  2. 排除以下 hop-by-hop 头，不复制（Go HTTP 客户端会自动处理）：`Content-Length`、`Host`、`Connection`、`Transfer-Encoding`。
  3. 单独设置 `Authorization: Bearer <apiKey>`，覆盖客户端原始 Authorization 头（如有）。

### 步骤 5：修改 Execute() 中响应剥前缀的条件

将 line 225-226 的条件从：

```
if isClaudeOAuthToken(apiKey) && !auth.ToolPrefixDisabled()
```

改为：

```
if isClaudeOAuthToken(apiKey) && !auth.ToolPrefixDisabled() && !nativePassthrough
```

native passthrough 时，响应体中的工具名不做任何剥前缀处理，原样透传给下游。

### 步骤 6：在 ExecuteStream() 中执行对称修改

在 `ExecuteStream()` 方法中，按照与 Execute() 完全一致的逻辑执行以下修改：

1. 在方法入口（确定 `apiKey` 之后、line 280 的 `applyCloaking` 之前）计算 `nativePassthrough`，方式同步骤 1。
2. 用 `if !nativePassthrough` 包裹 line 294-296 的 `ensureCacheControl` 调用块（同步骤 2）。
3. 在 line 304-305 的 proxy_ 前缀条件中增加 `&& !nativePassthrough`（同步骤 3）。
4. 将 line 313 的 `applyClaudeHeaders(...)` 调用改为与步骤 4 相同的分支逻辑（注意流式调用时第四个参数为 `true`）。

### 步骤 7：修改 ExecuteStream() 中两处流式剥前缀的条件

`ExecuteStream()` 的 SSE 转发路径中有两处 `stripClaudeToolPrefixFromStreamLine` 调用：

- `from == to` 直接转发路径（line 377-378）
- 翻译路径（line 404-405）

两处均需在原有条件基础上增加 `&& !nativePassthrough`，确保 native passthrough 时 SSE 流中的工具名原样透传，不做剥前缀处理。

## 验证命令

运行 task-001-test 中定义的 5 个 BDD 测试场景（在 Red 阶段测试写完后执行）：

```
cd /home/adam/projects/CLIProxyAPI && go test ./internal/runtime/executor/ -run TestClaudeExecutor_NativePassthrough -v
```

所有 5 个测试应全部 PASS（Green）。

完整回归验证（确保没有破坏现有测试）：

```
cd /home/adam/projects/CLIProxyAPI && go test ./internal/runtime/executor/ -v
```
