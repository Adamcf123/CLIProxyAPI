# Task 001 (Red): PrintSummary 输出 provider 和 model 字段 — 测试

**depends-on**: —
**type**: test (Red)
**paired-impl**: task-001-print-summary-fields-impl.md

## Goal

在 `internal/metricsruntime/display_test.go` 中为 `PrintSummary()` 编写失败的单元测试，验证输出中包含 `provider` 和 `model` 行。

## BDD Scenario

```gherkin
Scenario: PrintSummary 包含 provider 和 model 字段
  Given 一个 RequestState，其 Provider="claude"，Model="claude-sonnet-4-6"，
        WindowStatsE2E.Count=5，TPSE2EAvg=12.345
  When 调用 PrintSummary(state)
  Then stderr 输出的第一行为 "metrics_summary provider=claude"
  And  stderr 输出的第二行为 "metrics_summary model=claude-sonnet-4-6"
  And  后续行仍包含 "metrics_summary request_count=5"、"metrics_summary time_window=10m"、"metrics_summary tps_avg=12.345"

Scenario: provider 或 model 为空时仍输出空值行
  Given 一个 RequestState，其 Provider=""，Model=""
  When 调用 PrintSummary(state)
  Then stderr 输出包含 "metrics_summary provider="
  And  stderr 输出包含 "metrics_summary model="
```

## Files to Create / Modify

- **Modify** (or create if absent): `internal/metricsruntime/display_test.go`
  - 新增测试函数 `TestPrintSummary_IncludesProviderAndModel`
  - 新增测试函数 `TestPrintSummary_EmptyProviderAndModel`
  - 将 `os.Stderr` 重定向到 `bytes.Buffer` 以捕获输出（或通过 `io.Writer` 注入）；若当前 `PrintSummary` 不支持 writer 注入，则直接测试 `os.Stderr` 管道

## Steps

1. 找到（或创建）`internal/metricsruntime/display_test.go`
2. 为 `TestPrintSummary_IncludesProviderAndModel` 编写测试：
   - 构造带有 Provider/Model 的 RequestState
   - 捕获 stderr 输出
   - 断言前两行分别为 `metrics_summary provider=claude` 和 `metrics_summary model=claude-sonnet-4-6`
   - 断言后三行内容与原有三行格式一致
3. 为 `TestPrintSummary_EmptyProviderAndModel` 编写测试：
   - Provider/Model 均为空字符串
   - 断言输出中存在 `metrics_summary provider=` 和 `metrics_summary model=` 行
4. 运行测试确认红色（测试应失败，因为 PrintSummary 尚未输出 provider/model）

## Verification

```bash
go test ./internal/metricsruntime/... -run TestPrintSummary -v
```

期望结果：测试 **失败**（Red），错误信息显示输出中缺少 provider/model 行。
