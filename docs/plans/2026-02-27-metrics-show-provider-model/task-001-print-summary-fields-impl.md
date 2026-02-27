# Task 001 (Green): PrintSummary 输出 provider 和 model 字段 — 实现

**depends-on**: task-001-print-summary-fields-test.md
**type**: impl (Green)
**paired-test**: task-001-print-summary-fields-test.md

## Goal

修改 `internal/metricsruntime/display.go` 中的 `PrintSummary()` 函数，在现有三行输出之前增加 `provider` 和 `model` 两行，使 task-001 的测试通过。

## BDD Scenario

```gherkin
Scenario: PrintSummary 包含 provider 和 model 字段
  Given 一个 RequestState，其 Provider="claude"，Model="claude-sonnet-4-6"
  When 调用 PrintSummary(state)
  Then stderr 输出的第一行为 "metrics_summary provider=claude"
  And  stderr 输出的第二行为 "metrics_summary model=claude-sonnet-4-6"
  And  后续行保持原有格式：request_count、time_window、tps_avg

Scenario: provider 或 model 为空时仍输出空值行
  Given 一个 RequestState，其 Provider=""，Model=""
  When 调用 PrintSummary(state)
  Then 输出包含 "metrics_summary provider=" 和 "metrics_summary model="
```

## Files to Modify

- **Modify**: `internal/metricsruntime/display.go`
  - 函数：`PrintSummary(state *RequestState)`（当前位于约第 54-64 行）
  - 在现有 `windowStatsE2E.Count` 输出行**之前**添加两行：
    - `metrics_summary provider=<snap.Provider>`
    - `metrics_summary model=<snap.Model>`
  - 不添加任何条件判断（空字符串直接输出）
  - 不修改其他函数或文件

## Steps

1. 读取 `internal/metricsruntime/display.go` 当前 `PrintSummary` 函数内容
2. 在获取 `snap` 之后、输出 `request_count` 之前，添加两行 `fmt.Fprintf(os.Stderr, "metrics_summary provider=%s\n", snap.Provider)` 和 `fmt.Fprintf(os.Stderr, "metrics_summary model=%s\n", snap.Model)`
3. 保持其他三行不变
4. 运行测试确认绿色

## Verification

```bash
go test ./internal/metricsruntime/... -run TestPrintSummary -v
```

期望结果：测试 **通过**（Green）。

```bash
go build ./...
```

期望结果：编译无错误。
