---
phase: quick-007-metrics-summary-include-provider-model-w
plan: 007
subsystem: metrics
tags: [metrics, metrics_summary, stderr, window, tps, ttft, tpot]

provides:
  - "stderr metrics_summary JSON adds window_stats (count + avg tps/ttft/tpot) keyed by provider/model/streaming"
  - "stderr metrics_summary JSON adds errors_total keyed by provider/model/streaming (failure only; excludes canceled=499)"

key_files:
  modified:
    - internal/metricsruntime/display.go
    - internal/metricsruntime/request_state.go
    - internal/metricsruntime/usage_plugin.go
    - internal/metricsruntime/display_test.go
    - internal/metricsruntime/usage_plugin_test.go

completed: 2026-02-02
---

# Quick Task 007: metrics_summary window_stats + errors_total

**在每次请求结束输出的 stderr 单行 `metrics_summary {json}` 中新增按 provider/model/streaming 维度的窗口吞吐统计（window_stats）与累计错误数（errors_total），用于直接从运行时证据判断近期健康度与吞吐趋势。**

## What Changed

- 运行时 summary 输出（`internal/metricsruntime/display.go`）追加两个新字段：
  - `window_stats`: `{count, tps_avg, ttft_avg, tpot_avg}`（窗口为空时 avg 为 `null`）
  - `errors_total`: 截止当前请求、按 provider/model/streaming 维度累计的失败次数
- RequestState 快照（`internal/metricsruntime/request_state.go`）新增对应字段，保证 `PrintSummary` 只依赖 snapshot（不直接访问 collector）
- usage 插件（`internal/metricsruntime/usage_plugin.go`）在每次 `HandleUsage` 后：
  - 从 `TPSCollector.GetWindowStats(key)` 读取窗口统计并写入当前 RequestState
  - 按 `snap.IsFailure() && !snap.IsClientCanceled()` 口径更新 errors_total（排除 canceled=499）

## Tests

- `go test ./...`

## Task Commits

- Task 1: metrics_summary 增加 window_stats + errors_total（保持旧字段不变）
  - `2c3e6f2` feat(quick-007-metrics-summary-include-provider-model-w): add window_stats and errors_total to metrics_summary
- Task 2: 更新 display_test / usage_plugin_test 覆盖 window_stats + errors_total 契约
  - `57528c6` test(quick-007-metrics-summary-include-provider-model-w): cover window_stats and errors_total

## Execution

- Started: 2026-02-02T09:07:36Z
- Completed: 2026-02-02T09:25:54Z
- Duration: 18 min
