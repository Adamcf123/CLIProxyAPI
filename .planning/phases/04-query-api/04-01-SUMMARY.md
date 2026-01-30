---
phase: 04-query-api
plan: 01
subsystem: database
tags: [sqlite, goose, metrics, streaming]

# Dependency graph
requires:
  - phase: 03-persistence
    provides: SQLite metrics persistence (goose migrations + async writer)
provides:
  - SQLite metrics rows now include streaming dimension (0/1) for filtering/grouping
  - Indexes supporting time-range scans and provider/model/streaming grouping
affects: [query-api, analytics, stor-02, stor-03, stor-04]

# Tech tracking
tech-stack:
  added: []
  patterns: ["SQLite schema evolves via additive goose migrations", "Async writer persists request-level metrics off request path"]

key-files:
  created:
    - internal/metricspersist/migrations/0002_add_streaming.sql
  modified:
    - internal/metricspersist/types.go
    - internal/metricsruntime/usage_plugin.go
    - internal/metricspersist/writer.go
    - internal/metricspersist/writer_test.go

key-decisions:
  - "Historical metrics rows default streaming=0 to avoid NULL group keys"
  - "Writer treats nil Streaming as 0 (fail-closed)"

patterns-established:
  - "Dimension persistence: runtime snapshot -> MetricRecord -> SQLite column (no feature flags)"

# Metrics
duration: 4min
completed: 2026-01-30
---

# Phase 4 Plan 01: Streaming Dimension Summary

**SQLite metrics persistence now stores a first-class streaming dimension (INTEGER 0/1) with indexes to unblock provider+model+streaming queries.**

## Performance

- **Duration:** 4 min
- **Started:** 2026-01-30T08:57:29Z
- **Completed:** 2026-01-30T09:01:30Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- SQLite schema（`metrics` 表）新增 `streaming` 列并提供可用的默认值（0），保证历史数据可被分组查询
- writer 写入链路从 runtime snapshot 传递并入库 streaming，避免 Query API 出现 NULL/缺失维度
- 增强 writer 测试，读回 `metrics.streaming` 并断言 0/1 映射正确

## Task Commits

每个 task 均已原子提交：

1. **Task 1: 新增 streaming 列（migration + 索引）** - `a0c8246` (feat)
2. **Task 2: 写入链路填充 streaming（MetricRecord + usage_plugin + writer）** - `41c29a2` (feat)

## Files Created/Modified

- `internal/metricspersist/migrations/0002_add_streaming.sql` - 增量迁移：新增 streaming 列 + 相关索引
- `internal/metricspersist/types.go` - `MetricRecord` 增加 `Streaming *bool`
- `internal/metricsruntime/usage_plugin.go` - enqueue 时写入 `MetricRecord.Streaming`
- `internal/metricspersist/writer.go` - INSERT 语句包含 streaming，并将 `*bool` 映射为 0/1
- `internal/metricspersist/writer_test.go` - 读回 streaming 并验证 0/1

## Decisions Made

- 历史数据无法可靠回填 streaming，因此 schema 采用 `NOT NULL DEFAULT 0`，确保查询分组不会出现 NULL key。
- writer 对 nil 的 `MetricRecord.Streaming` 采取 fail-closed：写入 0，避免 NULL 分组与契约漂移。

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Query API 现在可以在 SQLite 层按 `provider + model + streaming` 过滤/分组，不再被 schema 缺失阻断。

---
*Phase: 04-query-api*
*Completed: 2026-01-30*
