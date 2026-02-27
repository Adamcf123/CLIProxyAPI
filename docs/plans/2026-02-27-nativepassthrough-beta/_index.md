# Plan: nativePassthrough Beta Header 修复

**Alignment**: [alignment.md](./alignment.md)
**日期**: 2026-02-27

## 目标

修复 nativePassthrough 路径下 `Anthropic-Beta` 缺少 `oauth-2025-04-20` 导致 OAuth 认证失败的问题，并同步更新 `applyClaudeHeaders` 中过时的硬编码默认 beta 常量。

## 约束

- 三处 nativePassthrough 路径（Execute、ExecuteStream、CountTokens）均需修改
- 仅追加缺失的 `oauth-2025-04-20`，不替换 CLI 发送的其余 beta
- 默认 beta 常量完全按 CLI 2.1.62 抓包实测值更新，不保留过时 beta

## 执行计划

| Task | 文件 | 类型 | depends-on |
|------|------|------|------------|
| [Task 001: nativePassthrough beta 追加测试（Red）](./task-001-nativepassthrough-beta-test.md) | `claude_executor_test.go` | test | 无 |
| [Task 001: nativePassthrough beta 追加实现（Green）](./task-001-nativepassthrough-beta-impl.md) | `claude_executor.go` | impl | task-001-test |
| [Task 002: 默认 beta 常量更新测试（Red）](./task-002-default-beta-test.md) | `claude_executor_test.go` | test | 无 |
| [Task 002: 默认 beta 常量更新实现（Green）](./task-002-default-beta-impl.md) | `claude_executor.go` | impl | task-002-test |
| [Task 003: 创建 beta 参考文档](./task-003-beta-reference-docs.md) | `docs/claude-beta-reference.md` | impl | 无 |

## 并行执行提示

Task 001 测试、Task 002 测试、Task 003 可并行启动（互无依赖）。
Task 001 实现依赖 Task 001 测试完成；Task 002 实现依赖 Task 002 测试完成。
