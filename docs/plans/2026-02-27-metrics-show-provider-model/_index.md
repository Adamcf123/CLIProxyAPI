# Plan: metrics_summary 两个修复 + Claude token 字段完整性

**Alignment**: [alignment.md](./alignment.md)
**Date**: 2026-02-27

## Goal

修复三个独立问题：
1. **Bug 1**：`metrics_summary` 日志缺少 provider 和 model 字段
2. **Bug 2**：部分请求显示 `request_count=0` / `tps_avg=--`（E2E 条件过严）
3. **附加**：`parseClaudeUsage` / `parseClaudeStreamUsage` 缺失 ephemeral cache tokens 和 thinking_tokens

## Constraints

- 不修改 `RequestWindowStatsE2E` 结构体
- 不影响 management API 和 SQLite 持久化
- 所有改动必须通过现有测试（无回归）

## Files Affected

| 文件 | 修改原因 |
|------|---------|
| `internal/metricsruntime/display.go` | Bug 1：PrintSummary 添加 provider/model 输出 |
| `internal/metricsruntime/display_test.go` | Bug 1：新增测试 |
| `internal/metricsruntime/usage_plugin.go` | Bug 2：E2E 条件扩展为 generatedTokens |
| `internal/metricsruntime/usage_plugin_test.go` | Bug 2：新增测试 |
| `internal/runtime/executor/usage_helpers.go` | 附加：ephemeral cache tokens + thinking_tokens |
| `internal/runtime/executor/usage_helpers_test.go` | 附加：新增测试 |

## Execution Plan

三组 Red-Green 对，可并行执行（无跨组依赖）：

### Bug 1: PrintSummary provider/model

- [Task 001 Test (Red): PrintSummary provider/model 测试](./task-001-print-summary-fields-test.md)
- [Task 001 Impl (Green): PrintSummary provider/model 实现](./task-001-print-summary-fields-impl.md)

### Bug 2: E2E generated tokens 条件

- [Task 002 Test (Red): E2E generated tokens 条件测试](./task-002-e2e-generated-tokens-test.md)
- [Task 002 Impl (Green): E2E generated tokens 条件实现](./task-002-e2e-generated-tokens-impl.md)

### 附加: Claude token 字段完整性

- [Task 003 Test (Red): parseClaudeUsage ephemeral cache tokens 测试](./task-003-claude-ephemeral-cache-test.md)
- [Task 003 Impl (Green): parseClaudeUsage ephemeral cache tokens 实现](./task-003-claude-ephemeral-cache-impl.md)

## Dependency Graph

```
task-001-test  ──►  task-001-impl
task-002-test  ──►  task-002-impl
task-003-test  ──►  task-003-impl
```

三组之间无依赖，可并行执行所有 Red 任务，再并行执行所有 Green 任务。
