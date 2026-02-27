# Task 001: nativePassthrough beta 追加测试（Red）

**Type**: test
**depends-on**: 无
**Target file**: `internal/runtime/executor/claude_executor_test.go`

## BDD Scenario

Scenario: nativePassthrough 路径在 beta 缺失 oauth 时自动追加
  Given 代理上游配置了 OAuth token（sk-ant-oat 前缀）
  And 客户端 User-Agent 为 claude-cli 开头
  And 格式无需转换（from == to == claude）
  And 客户端发送的 Anthropic-Beta 不含 oauth-2025-04-20
  When 请求通过 Execute / ExecuteStream / CountTokens 路径处理
  Then 上游收到的 Anthropic-Beta 应包含 oauth-2025-04-20
  And 上游收到的其余 beta 应与客户端发送的完全一致（不丢失）

Scenario: nativePassthrough 路径在 beta 已含 oauth 时不重复追加
  Given 代理上游配置了 OAuth token
  And nativePassthrough 触发条件满足
  And 客户端发送的 Anthropic-Beta 已含 oauth-2025-04-20
  When 请求通过任一 nativePassthrough 路径处理
  Then 上游收到的 Anthropic-Beta 与客户端发送值完全相同（无重复）

## Steps

1. 在 `claude_executor_test.go` 中新增测试函数 `TestClaudeExecutor_NativePassthrough_OAuthBeta`
2. 参考现有 `TestClaudeExecutor_NativePassthrough_BetaHeaderUnchanged` 的测试结构（mock upstream server + gin context 注入）
3. 测试 case 1：客户端发送 `Anthropic-Beta: claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28`（无 oauth），验证上游收到包含 oauth-2025-04-20 的 beta
4. 测试 case 2：客户端发送已含 `oauth-2025-04-20` 的 beta，验证上游收到的 beta 与原值一致
5. 覆盖三个路径：Execute、ExecuteStream（流式）、CountTokens
6. 验证：新测试在当前代码下失败（Red），现有测试不受影响

## Verification

```bash
# 新测试应失败（Red phase）
go test ./internal/runtime/executor/... -run TestClaudeExecutor_NativePassthrough_OAuthBeta -v

# 现有测试应通过（不破坏已有测试）
go test ./internal/runtime/executor/... -run TestClaudeExecutor_NativePassthrough -v
```
