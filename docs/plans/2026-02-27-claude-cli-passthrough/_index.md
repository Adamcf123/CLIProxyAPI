# Plan: Claude CLI 完全透传

## 目标

当上游客户端是 Claude Code CLI（User-Agent 以 `claude-cli` 开头）且使用 OAuth token 时，
代理进入 **native passthrough 模式**：仅替换 Authorization token，不对请求体和其他头部做任何修改。

## 背景

通过 mitmproxy 对 claude-cli/2.1.62 的抓包证实，cli 的请求已经是自洽的——
自带正确的 Anthropic-Beta 列表、cache_control、真实 user_id。
代理的修改（proxy_ 工具前缀、beta 追加、额外头部注入）是为第三方客户端设计的，
对 cli 客户端既无必要，又引入偏差。

详见：`docs/plans/2026-02-27-claude-cli-passthrough/alignment.md`

## 约束

- 仅修改 `internal/runtime/executor/claude_executor.go` 和对应测试文件
- 第三方客户端（非 claude-cli UA）的现有行为不受影响
- 指标采集继续正常工作
- 对 `applyClaudeHeaders` 函数本身零侵入

## Execution Plan

- [Task 001 (Red): Native Passthrough 测试](./task-001-native-passthrough-test.md)
- [Task 001 (Green): 实现 Native Passthrough](./task-001-native-passthrough-impl.md)
