---
phase: quick
plan: 008-metrics-summary-align-window-stats-error
subsystem: metrics
tags: [metrics_summary, window_stats, errors_total, request_state, tpscollector]

# Dependency graph
requires:
  - phase: quick
    provides: metrics_summary includes window_stats/errors_total (quick 007)
provides:
  - Align window_stats/errors_total aggregation key to metrics_summary provider/model/streaming
  - Regression tests locking in key alignment and SQLite provider/model persistence semantics
affects: [runtime_validation, troubleshooting]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Centralized MetricKey derivation from RequestStateSnapshot + usage.Record (state-preferred, record fallback)

key-files:
  created:
    - .planning/quick/008-metrics-summary-align-window-stats-error/008-SUMMARY.md
  modified:
    - internal/metricsruntime/usage_plugin.go
    - internal/metricsruntime/usage_plugin_test.go
    - .planning/STATE.md

key-decisions:
  - "For runtime evidence integrity, window_stats/errors_total aggregation uses RequestStateSnapshot provider/model/streaming; usage record provider/model only fills missing snapshot values; SQLite persistence remains record-sourced."

patterns-established:
  - "metricsruntime MetricKey selection is a single helper (metricKeyFromStateOrRecord) to avoid drift between CompleteRequest and GetWindowStats/updateErrorsTotal."

# Metrics
duration: 5min
completed: 2026-02-02
---

# Quick Task 008: metrics_summary window_stats/errors_total Key Alignment

**stderr `metrics_summary` 的 `window_stats` / `errors_total` 现在按与同一行 `provider`/`model` 展示一致的维度聚合（provider/model/streaming，state 优先、record 回退）。**

## Performance

- **Duration:** 5 min
- **Started:** 2026-02-02T10:47:41Z
- **Completed:** 2026-02-02T10:52:10Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- 统一聚合 key（`internal/metricsruntime/usage_plugin.go`）→ `CompleteRequest`、`GetWindowStats`、`errors_total` 递增使用同一套 state 对齐规则
- 回归测试锁定契约（`internal/metricsruntime/usage_plugin_test.go`）→ state 覆盖 record 分组、且 SQLite 落库 provider/model 语义不变

## Task Commits

Each task was committed atomically:

1. **Task 1: 统一 window_stats/errors_total 的聚合 key（state 优先，record 回退）** - `72d9133` (feat)
2. **Task 2: 补齐回归测试：对齐 key + 不影响落库 provider/model** - `e3e66a3` (test)

## Files Created/Modified

- `internal/metricsruntime/usage_plugin.go` - 统一计算与 metrics_summary 展示对齐的 `metrics.MetricKey`（state 优先，record 回退）
- `internal/metricsruntime/usage_plugin_test.go` - 回归：collector window_stats/errors_total key 与 state 展示对齐；且落库 provider/model 不被影响
- `.planning/quick/008-metrics-summary-align-window-stats-error/008-SUMMARY.md` - 本次 quick task 执行记录
- `.planning/STATE.md` - 追加 quick task 008 记录

## Decisions Made

None - followed plan as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- runtime evidence（stderr `metrics_summary`）不再出现 provider/model 与 window_stats/errors_total 维度错位，便于直接 grep 定位问题

---
*Phase: quick*
*Completed: 2026-02-02*
