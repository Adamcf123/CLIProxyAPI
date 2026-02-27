# Task 001 (Red): Native Passthrough 测试

**depends-on:** 无

**类型:** 测试（Red 阶段）

**关联任务:**
- task-001-native-passthrough-test.md（本文件，Red）
- task-001-native-passthrough-impl.md（Green，depends-on 本文件）

---

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

---

## 要修改的文件

- `internal/runtime/executor/claude_executor_test.go`

---

## 步骤描述

**步骤 1 — 新增辅助函数 `makeGinCtxWithUA`**

在 `claude_executor_test.go` 中新增辅助函数 `makeGinCtxWithUA(ua, betaHeader string)`，用于构建携带指定 User-Agent 和 Anthropic-Beta 头的 gin 测试 context。该函数使用 `httptest.NewRecorder()` 和 `gin.CreateTestContext()` 创建测试用 gin context，并将其通过 `context.WithValue` 以 `"gin"` 为 key 封装到 `context.Context` 中返回，参照 `internal/metricsruntime/usage_plugin_test.go:44` 的已有先例。

**步骤 2 — 新增测试函数 `TestClaudeExecutor_NativePassthrough_BodyUnchanged`（场景 A）**

启动 `httptest.NewServer`，在 handler 中读取并解析 `r.Body`，提取请求体中 `tools` 数组的各工具名称，将其存入 channel 供后续断言使用。使用 OAuth token（api_key 含 `"sk-ant-oat"`）和 claude-cli UA 构建 gin context，构造包含工具 `"Read"` 和 `"Write"` 的请求体，调用 `Execute()`。断言上游收到的工具名列表中存在 `"Read"` 和 `"Write"`，且不存在 `"proxy_Read"` 或 `"proxy_Write"`。

**步骤 3 — 新增测试函数 `TestClaudeExecutor_NativePassthrough_BetaHeaderUnchanged`（场景 B）**

启动 `httptest.NewServer`，在 handler 中记录 `r.Header.Get("anthropic-beta")` 的值。使用 OAuth token 和 claude-cli UA 构建 gin context，将 gin context 的请求头设置 `Anthropic-Beta` 为 `"claude-code-20250219,oauth-2025-04-20,prompt-caching-scope-2026-01-05"`，调用 `Execute()`。断言上游收到的 `anthropic-beta` 头值与客户端发送的原始值完全一致，不含额外追加的 beta 条目（如 `"prompt-caching-2024-07-31"`）。

**步骤 4 — 新增测试函数 `TestClaudeExecutor_NativePassthrough_NoStainlessHelperMethod`（场景 C）**

启动 `httptest.NewServer`，在 handler 中记录 `r.Header.Get("X-Stainless-Helper-Method")` 的值。使用 OAuth token 和 claude-cli UA 构建 gin context，gin context 的请求头中不设置 `X-Stainless-Helper-Method`，调用 `Execute()`。断言上游收到的请求中 `X-Stainless-Helper-Method` 头为空字符串。

**步骤 5 — 新增测试函数 `TestClaudeExecutor_NativePassthrough_StreamResponseUnchanged`（场景 D）**

启动 `httptest.NewServer`，在 handler 中返回包含 `"name":"proxy_Read"` 的 SSE 流（Content-Type 为 `text/event-stream`，每行格式为 `data: {...}`）。使用 OAuth token 和 claude-cli UA 构建 gin context，调用 `ExecuteStream()`，收集所有流输出行。断言收到的所有 SSE 数据行中包含字符串 `"proxy_Read"`，且不包含单独出现的 `"Read"` 作为工具名（即前缀未被剥除）。

**步骤 6 — 新增测试函数 `TestClaudeExecutor_NativePassthrough_ThirdPartyClientUnaffected`（场景 E）**

启动 `httptest.NewServer`，在 handler 中读取并解析 `r.Body`，提取请求体中工具名称。使用 OAuth token 和 `cursor/1.0` UA 构建 gin context，构造包含工具 `"Read"` 的请求体，调用 `Execute()`。断言上游收到的工具名为 `"proxy_Read"`（即在非 claude-cli UA 时 proxy_ 前缀仍正常应用，回归保护通过）。

---

## 验证命令

```bash
cd /home/adam/projects/CLIProxyAPI && go test ./internal/runtime/executor/ -run TestClaudeExecutor_NativePassthrough -v
```

此命令在 Green 阶段实现前应全部 FAIL（Red）。所有 5 个测试函数均应报告 `FAIL` 或编译错误，确认测试先于实现存在。
