---
phase: 04-query-api
plan: 04
subsystem: api
tags: [sqlite, gin, metrics, buckets, utc, aggregation]

# Dependency graph
requires:
  - phase: 04-query-api/04-03
    provides: mode=percentiles success/failure split rule + shared unit conversion helper
provides:
  - "GET /v0/management/metrics?mode=buckets&bucket=..." fixed-granularity time-series aggregation with UTC-aligned buckets and empty-bucket fill
affects: [dashboards, trend-analysis, STOR-04]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "SQLite coarse aggregation with unixepoch(created_at) bucketization + Go-side axis fill"
    - "Explicit success/failure split via hard-coded CASE expression to prevent drift"

key-files:
  created: []
  modified:
    - internal/api/handlers/management/metrics.go
    - test/metrics_management_test.go

key-decisions:
  - "Bucket presets locked to: 1m/5m/15m/1h/1d; invalid preset returns 400"
  - "Buckets align to UTC natural boundaries; meta echoes timezone=UTC and effective_from/effective_to"

patterns-established:
  - "Empty bucket contract: count=0 + all metric fields null (success/failure returned separately)"

# Metrics
duration: 8m
completed: 2026-01-30
---

# Phase 4 Plan 04: Buckets Mode Summary

**mode=buckets：按固定粒度（1m/5m/15m/1h/1d）输出 UTC 对齐的时间序列，并在 Go 侧为每个分组回填空 bucket（count=0 + 指标为 null）**

## Performance

- **Duration:** 7m 31s
- **Started:** 2026-01-30T09:27:29Z
- **Completed:** 2026-01-30T09:35:00Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- buckets 查询（`internal/api/handlers/management/metrics.go`）支持固定粒度 + UTC 自然边界对齐，并在 `meta` 回显 `bucket/timezone/requested/effective/filters`
- buckets 聚合在 SQLite 侧做粗聚合（按 provider/model/streaming + success_flag + bucket_start），Go 侧按分组生成完整时间轴并回填空 bucket
- buckets 契约测试（`test/metrics_management_test.go`）锁定对齐规则、空 bucket 行为、以及秒→毫秒整数的取整一致性（`0.0016s -> 2ms`）

## Task Commits

Each task was committed atomically:

1. **Task 1: 实现 mode=buckets（固定粒度 + UTC 对齐 + SQL 粗聚合）** - `047eaf2` (feat)
2. **Task 2: 增加 buckets 契约测试（对齐 + 空 bucket + 单位）** - `6be8eb6` (test)

## Files Created/Modified
- `internal/api/handlers/management/metrics.go` - buckets 分支（bucket 校验、UTC 对齐、SQL 聚合、Go 侧空 bucket 回填、meta 回显）
- `test/metrics_management_test.go` - buckets 契约测试（对齐 + 空 bucket + 毫秒取整）

## Decisions Made
- None - followed plan as specified.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] 修复 buckets 查询的时间过滤以支持 RFC3339 created_at（含 'T'/'Z'）**
- **Found during:** Task 2 (buckets 契约测试)
- **Issue:** buckets SQL 用 `created_at >= datetime(?)` 过滤时，RFC3339 格式（`2026-...T...Z`）在 SQLite 比较中会被错误排除，导致聚合结果为空。
- **Fix:** 改为用 `unixepoch(created_at) >= unixepoch(?)` 的数值比较，避免格式差异导致的比较偏差。
- **Files modified:** internal/api/handlers/management/metrics.go
- **Verification:** `go test ./...`
- **Committed in:** `9e407b5`

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** 修复为 buckets correctness 的必要条件，无范围扩张。

## Issues Encountered
- SQLite 时间比较在不同时间字符串格式（`CURRENT_TIMESTAMP` vs RFC3339）下出现隐式行为差异；已通过统一使用 `unixepoch(...)` 进行数值比较解决。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 4（Query API）计划项已补齐：percentiles + buckets + request_id 查询均具备可测试的稳定 contract。

---
*Phase: 04-query-api*
*Completed: 2026-01-30*
