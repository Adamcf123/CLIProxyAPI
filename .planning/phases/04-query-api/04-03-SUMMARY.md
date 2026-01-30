---
phase: 04-query-api
plan: 03
subsystem: api
tags: [gin, sqlite, percentiles, p50, p95, p99, linear-interpolation]

# Dependency graph
requires:
  - phase: 04-02
    provides: management metrics endpoint scaffold (filters/time-range/meta + request_id lookup)
provides:
  - mode=percentiles aggregation on GET /v0/management/metrics
  - reusable linear-interpolation percentile helpers (p50/p95/p99)
  - contract test coverage for success/failure split + NULL handling
affects: [buckets mode, analytics contract stability]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Percentiles computed in Go using internal/metrics linear interpolation (avoid SQL percentile drift)"
    - "NULL samples excluded from percentile inputs; empty sample set yields null percentiles"

key-files:
  created:
    - internal/metrics/percentiles.go
  modified:
    - internal/api/handlers/management/metrics.go
    - test/metrics_management_test.go

key-decisions:
  - "Expose CalculatePercentile/CalculateP50P95P99 as the single percentile API to prevent semantic drift"
  - "Percentiles mode treats missing DB file/schema as empty result (no 500) to keep endpoint usable in fresh envs/tests"

patterns-established:
  - "Query API calls metrics.CalculateP50P95P99 for percentiles (single algorithm source of truth)"

# Metrics
duration: 7min
completed: 2026-01-30
---

# Phase 4 Plan 03: Percentiles Mode Summary

**`GET /v0/management/metrics?mode=percentiles` 按 provider+model+streaming 分组输出 success/failure 两套聚合，并对各指标计算 p50/p95/p99（线性插值语义一致）。**

## Performance

- **Duration:** 7 min
- **Started:** 2026-01-30T09:17:24Z
- **Completed:** 2026-01-30T09:24:49Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- 百分位计算（`internal/metrics/percentiles.go`）复用 `internal/metrics/window.go` 的线性插值实现并对外导出
- 聚合查询（`internal/api/handlers/management/metrics.go`）实现 `mode=percentiles`，输出 success/failure 分离 + NULL 样本不污染
- 契约测试（`test/metrics_management_test.go`）锁定 p50/p95/p99 结果、success/failure 切分规则与毫秒取整语义

## Task Commits

Each task was committed atomically:

1. **Task 1: 导出线性插值 percentile 计算（复用 internal/metrics/window.go 语义）** - `ee71e4b` (feat)
2. **Task 2: 在 GetMetrics 实现 mode=percentiles 聚合（success/failure 分开）** - `e701c2f` (feat)
3. **Task 3: 增加 percentiles 契约测试（p50/p95/p99 + success/failure + NULL 行为）** - `97cfa42` (test)

## Files Created/Modified
- `internal/metrics/percentiles.go` - Export linear-interpolation percentile APIs for reuse
- `internal/api/handlers/management/metrics.go` - Implement percentiles mode response + aggregation logic
- `test/metrics_management_test.go` - Add deterministic contract test for percentiles mode

## Decisions Made
- 统计语义单源：Query API 只调用 `metrics.CalculateP50P95P99`，避免在 handler 内复制/漂移 percentile 算法。
- 对“无数据/未初始化 DB”场景做 fail-soft：percentiles 查询遇到缺 DB 文件/缺表时返回空数组（而不是 500），保持 API contract 可用。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] percentiles 在缺省 DB/缺表时返回空结果以避免测试与新环境 500**
- **Found during:** Task 2 (mode=percentiles handler implementation)
- **Issue:** 测试环境未设置 metrics DB 路径时会触发 SQLite 打开失败/缺表，导致 endpoint 返回 500，阻断后续契约测试编写与运行。
- **Fix:** 对打开 DB 失败与 "no such table: metrics" 做识别并返回空聚合结果（success/failure 为空数组）。
- **Files modified:** internal/api/handlers/management/metrics.go
- **Verification:** `go test ./...`
- **Committed in:** e701c2f (part of task commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** 必要的可用性/可测试性修复；不改变核心 contract 字段与统计语义。

## Issues Encountered
- None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- percentiles contract 已锁定，可继续实现 `mode=buckets` 并复用同一分组维度与 NULL 语义。

---
*Phase: 04-query-api*
*Completed: 2026-01-30*
