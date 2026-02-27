# Task 002 (Green): E2E 条件支持 generatedTokens>0 — 实现

**depends-on**: task-002-e2e-generated-tokens-test.md
**type**: impl (Green)
**paired-test**: task-002-e2e-generated-tokens-test.md

## Goal

修改 `internal/metricsruntime/usage_plugin.go` 中的 E2E 条件（第 261 行附近），将 `outputTokens > 0` 替换为 `generatedTokens > 0`（其中 `generatedTokens = outputTokens + reasoningTokens`），使 task-002 的测试通过。

## BDD Scenario

```gherkin
Scenario: outputTokens=0 但有生成 token 时 E2E 被记录
  Given usage.Record 中 OutputTokens=0，ReasoningTokens>0 或 TotalTokens>InputTokens
  When  HandleUsage 处理该 record
  Then  WindowStatsE2E.Count > 0

Scenario: outputTokens>0 时 E2E 仍正常记录（回归）
  Given usage.Record 中 OutputTokens=20，Failed=false
  When  HandleUsage 处理该 record
  Then  WindowStatsE2E.Count > 0
```

## Files to Modify

- **Modify**: `internal/metricsruntime/usage_plugin.go`
  - 找到当前的 E2E 条件（约第 261 行）：
    ```go
    if state != nil && hasKey && !isCanceled && outputTokens > 0 && durationMs > 0 {
    ```
  - 在此条件之前计算 `generatedTokens`：
    ```
    generatedTokens = outputTokens + reasoningTokens
    （若 generatedTokens == 0，备用：取 max(0, totalTokens - inputTokens)）
    ```
  - 将条件中的 `outputTokens > 0` 替换为 `generatedTokens > 0`
  - TPS 计算中将 `float64(outputTokens)` 替换为 `float64(generatedTokens)`（同一代码块内约第 268-273 行）
  - 注意：`reasoningTokens` 需要从 `record.Detail.ReasoningTokens` 提取（与 `inputTokens`/`outputTokens` 同一提取位置）

## Steps

1. 读取 `internal/metricsruntime/usage_plugin.go`，定位：
   - `inputTokens`/`outputTokens`/`totalTokens` 的提取位置（`HandleUsage` 函数内）
   - E2E 条件行（`outputTokens > 0`）
   - TPS 计算行（`float64(outputTokens) / secs`）
2. 在 token 提取位置旁边添加 `reasoningTokens := record.Detail.ReasoningTokens`
3. 计算 `generatedTokens := outputTokens + reasoningTokens`，若为 0 则备用 `max(0, totalTokens-inputTokens)`
4. 将 E2E 条件中的 `outputTokens > 0` 改为 `generatedTokens > 0`
5. 将 TPS 计算中的 `float64(outputTokens)` 改为 `float64(generatedTokens)`
6. 运行测试确认绿色

## Verification

```bash
go test ./internal/metricsruntime/... -run TestHandleUsage_E2E -v
```

期望结果：测试 **通过**（Green）。

```bash
go test ./internal/metricsruntime/...
```

期望结果：所有现有测试通过（无回归）。

```bash
go build ./...
```

期望结果：编译无错误。
