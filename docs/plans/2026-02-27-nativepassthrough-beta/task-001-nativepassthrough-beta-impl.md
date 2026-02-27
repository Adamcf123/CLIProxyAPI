# Task 001: nativePassthrough beta 追加实现（Green）

**Type**: impl
**depends-on**: task-001-nativepassthrough-beta-test
**Target file**: `internal/runtime/executor/claude_executor.go`

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
  Given nativePassthrough 触发条件满足
  And 客户端发送的 Anthropic-Beta 已含 oauth-2025-04-20
  When 请求通过任一 nativePassthrough 路径处理
  Then 上游收到的 Anthropic-Beta 与客户端发送值完全相同

## Steps

1. 在 `claude_executor.go` 中找到三处 nativePassthrough header 复制循环（Execute ~170、ExecuteStream ~354、CountTokens ~526）
2. 在每处循环结束后、`httpReq.Header.Set("Authorization", "Bearer "+apiKey)` 之前插入 oauth beta 追加逻辑：
   - 读取当前 `Anthropic-Beta` header 值
   - 若 `isClaudeOAuthToken(apiKey)` 为 true 且 beta 不含字符串 "oauth"，将 `,oauth-2025-04-20` 追加到 beta 值末尾
   - 将修改后的 beta 值 Set 回请求 header
3. 逻辑与 `applyClaudeHeaders` 约第 950-952 行的 oauth beta 确保模式一致，保持代码风格统一
4. 三处修改内容完全相同，仅所在函数不同（Execute、ExecuteStream、CountTokens）

## Verification

```bash
# 所有 nativePassthrough 测试应通过（Green）
go test ./internal/runtime/executor/... -run TestClaudeExecutor_NativePassthrough -v

# 完整测试套件不回归
go test ./internal/runtime/executor/... -v
```
