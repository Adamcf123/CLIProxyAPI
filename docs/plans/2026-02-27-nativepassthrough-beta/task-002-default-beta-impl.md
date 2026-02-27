# Task 002: applyClaudeHeaders 默认 beta 常量更新（Green）

**Type**: impl
**depends-on**: task-002-default-beta-test
**Target file**: `internal/runtime/executor/claude_executor.go`

## BDD Scenario

Scenario: 第三方客户端未发送 Anthropic-Beta 时使用更新后的默认 beta
  Given 代理上游配置了 OAuth token
  And 客户端 User-Agent 不是 claude-cli 开头（第三方客户端，非 nativePassthrough 路径）
  And 客户端请求不包含 Anthropic-Beta header
  When 请求通过 applyClaudeHeaders 路径处理
  Then 上游收到的 Anthropic-Beta 应精确等于：
       claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28

## Steps

1. 在 `claude_executor.go` 约第 947-948 行找到 `baseBetas` 的硬编码默认赋值
2. 将默认值替换为 2.1.62 抓包实测字符串：`claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28`
3. 检查约第 946 行的 `promptCachingBeta` 变量（值为 `prompt-caching-2024-07-31`）及其在 append 逻辑中的使用：若 baseBetas 新值已不包含此 beta，该 append 条件将无法匹配导致再追加过期 beta；需评估并移除或更新该 append 逻辑，确保最终 beta 与新值精确一致
4. 不修改其他逻辑（oauth beta append、extraBetas 合并等保持不变）

## Verification

```bash
# 更新后测试应通过（Green）
go test ./internal/runtime/executor/... -run TestClaudeExecutor_DefaultBeta -v

# 完整回归测试
go test ./internal/runtime/executor/... -v
```
