---
phase: 06-guaranteed-usage-publish
plan: 04
subsystem: testing
tags: [go, sqlite, goose, gin]

# Dependency graph
requires:
  - phase: 03-persistence
    provides: metricspersist InitDB/Migrate + SQLite writer schema
  - phase: 06-guaranteed-usage-publish
    provides: ensurePublished publishes at least one usage record per request
provides:
  - SQLite regression test locking no-usage row persistence + NULL token semantics
affects: [06-05, query-api]

# Tech tracking
tech-stack:
  added: []
  patterns: ["Override enqueueMetricRecord seam in tests to synchronously insert into SQLite"]

key-files:
  created: [internal/metricsruntime/guaranteed_usage_publish_test.go]
  modified: []

key-decisions:
  - "None - followed plan as specified"

patterns-established:
  - "Regression tests query SQLite directly for NULL vs 0 semantics"

# Metrics
duration: 1 min
completed: 2026-01-30
---

# Phase 6 Plan 04: Guaranteed Usage Publish Summary

**SQLite 回归测试锁定：即使上游没有 usage/tokens，仍会落一行可按 request_id 查询的记录，并且 tokens/TPS/TPOT 以 NULL 表达“缺失/未知”。**

## Performance

- **Duration:** 1 min
- **Started:** 2026-01-30T15:23:01Z
- **Completed:** 2026-01-30T15:24:58Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments

- 新增 SQLite 回归测试（`internal/metricsruntime/guaranteed_usage_publish_test.go`）验证 no-usage 成功请求仍可通过 `request_id` 查到 DB 行
- 锁定 NULL 语义：`input_tokens`/`output_tokens`/`total_tokens`/`tps`/`tpot` 必须为 NULL（不是 0）
- 锁定 RequestState 可提供的审计维度：`status_code=200`、`streaming=1`，且 `error_info` 为 NULL

## Task Commits

Each task was committed atomically:

1. **Task 1: Add SQLite regression test for no-usage persistence** - `9440d5c` (test)

**Plan metadata:** (added in final docs commit)

## Files Created/Modified

- `internal/metricsruntime/guaranteed_usage_publish_test.go` - 通过覆盖 `enqueueMetricRecord` 将 MetricRecord 同步写入临时 SQLite 并直接查询断言 NULL 语义

## Decisions Made

None - followed plan as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Ready for `06-05-PLAN.md` to cover executor wiring regression (stream end must ensurePublished).

---
*Phase: 06-guaranteed-usage-publish*
*Completed: 2026-01-30*
