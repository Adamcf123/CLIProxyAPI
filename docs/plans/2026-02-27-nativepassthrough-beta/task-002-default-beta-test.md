# Task 002: applyClaudeHeaders 默认 beta 常量测试（Red）

**Type**: test
**depends-on**: 无
**Target file**: `internal/runtime/executor/claude_executor_test.go`

## BDD Scenario

Scenario: 第三方客户端未发送 Anthropic-Beta 时使用更新后的默认 beta
  Given 代理上游配置了 OAuth token
  And 客户端 User-Agent 不是 claude-cli 开头（第三方客户端，非 nativePassthrough 路径）
  And 客户端请求不包含 Anthropic-Beta header
  When 请求通过 applyClaudeHeaders 路径处理
  Then 上游收到的 Anthropic-Beta 应精确等于 2.1.62 抓包实测值：
       claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28

## Steps

1. 在 `claude_executor_test.go` 中新增测试函数 `TestClaudeExecutor_DefaultBeta`
2. 模拟第三方客户端场景：User-Agent 非 `claude-cli` 开头，无 `Anthropic-Beta` header，上游 token 为 OAuth token
3. 捕获上游收到的 `Anthropic-Beta` header 值
4. 断言其精确等于 `claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28`
5. 确认当前代码下测试失败（Red），因为当前硬编码值包含 `fine-grained-tool-streaming-2025-05-14` 和 `prompt-caching-2024-07-31`

## Verification

```bash
# 新测试应失败（Red phase）
go test ./internal/runtime/executor/... -run TestClaudeExecutor_DefaultBeta -v

# 现有测试应通过（不破坏已有测试）
go test ./internal/runtime/executor/... -v
```
