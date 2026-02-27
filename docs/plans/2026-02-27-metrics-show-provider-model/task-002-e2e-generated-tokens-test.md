# Task 002 (Red): E2E 条件支持 generatedTokens>0 — 测试

**depends-on**: —
**type**: test (Red)
**paired-impl**: task-002-e2e-generated-tokens-impl.md

## Goal

在 `internal/metricsruntime/usage_plugin_test.go` 中添加失败的单元测试，验证当 `outputTokens=0` 但 `inputTokens>0`（或 `totalTokens>inputTokens`）时，`HandleUsage` 仍会调用 `SetWindowStatsE2E` 并使 count > 0。

## Background

当前 `usage_plugin.go:261` 的条件为 `outputTokens > 0`。对于纯工具调用或极短响应（`output_tokens=0` 但有 `input_tokens`），这个条件失败导致 E2E 统计被跳过，`request_count=0`。

正确行为：当 `generatedTokens = outputTokens + reasoningTokens > 0`，或 `totalTokens - inputTokens > 0` 时，应触发 E2E 统计。

## BDD Scenario

```gherkin
Scenario: outputTokens=0 但 totalTokens>inputTokens 时 E2E 应被记录
  Given 一个有效的 gin 上下文，provider="claude"，model="claude-sonnet-4-6"，streaming=true
  And   一个 usage.Record，其中 InputTokens=50，OutputTokens=0，TotalTokens=70，Failed=false
  And   generatedTokens = TotalTokens - InputTokens = 20 > 0
  And   durationMs > 0（请求已正常完成）
  When  HandleUsage 处理该 record
  Then  RequestState.WindowStatsE2E.Count > 0

Scenario: outputTokens=0 且 totalTokens=inputTokens 时 E2E 不应被记录（无生成）
  Given 一个有效的 gin 上下文，provider="claude"，model="claude-sonnet-4-6"
  And   一个 usage.Record，其中 InputTokens=50，OutputTokens=0，TotalTokens=50，Failed=false
  And   generatedTokens = TotalTokens - InputTokens = 0
  When  HandleUsage 处理该 record
  Then  RequestState.WindowStatsE2E.Count == 0（无生成 token 不应触发 TPS 计算）

Scenario: outputTokens>0 时 E2E 仍正常记录（回归）
  Given 一个有效的 gin 上下文，provider="claude"，model="claude-sonnet-4-6"
  And   一个 usage.Record，其中 OutputTokens=20，Failed=false
  When  HandleUsage 处理该 record
  Then  RequestState.WindowStatsE2E.Count > 0
```

## Files to Create / Modify

- **Modify**: `internal/metricsruntime/usage_plugin_test.go`
  - 新增测试函数 `TestHandleUsage_E2EWithZeroOutputTokens`（场景 1：outputTokens=0，totalTokens>inputTokens）
  - 新增测试函数 `TestHandleUsage_E2ESkippedWhenNoGenerated`（场景 2：纯 echo，无生成）
  - 新增测试函数 `TestHandleUsage_E2EWithNonZeroOutputTokens`（场景 3：回归测试）

## Steps

1. 找到 `internal/metricsruntime/usage_plugin_test.go`，了解现有测试的构造方式（gin 上下文创建、state 注入等）
2. 参照现有测试写法，构造 OutputTokens=0、TotalTokens>InputTokens 的 usage.Record
3. 调用 HandleUsage 并检查 state.WindowStatsE2E.Count
4. 运行测试确认红色（测试应失败）

## Verification

```bash
go test ./internal/metricsruntime/... -run TestHandleUsage_E2E -v
```

期望结果：测试 **失败**（Red），因为当前条件 `outputTokens > 0` 对 OutputTokens=0 的场景不通过。
