---
phase: 04-query-api
plan: 02
subsystem: api
tags: [go, gin, sqlite, metrics, management, query-api]

# Dependency graph
requires:
  - phase: 04-query-api/04-01
    provides: streaming dimension in SQLite + writer persistence
provides:
  - GET /v0/management/metrics with request_id single-record lookup
  - Fail-fast query validation (mode/from/to/streaming) with meta echo
  - Shared seconds->milliseconds rounding helper for latency fields
  - Contract tests for request_id branch + validation + default 1h meta
affects: [percentiles, buckets, query-api-contract]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Dedicated SQLite read handle for query endpoints (do not reuse writer DB)
    - Stable meta echo via injectable nowUTC clock

key-files:
  created:
    - internal/api/handlers/management/metrics.go
    - internal/api/handlers/management/metrics_units.go
    - test/metrics_management_test.go
  modified:
    - internal/api/server.go
    - internal/api/handlers/management/handler.go

key-decisions:
  - "This endpoint returns an envelope with meta+data; errors include an error string and still include meta when available"
  - "Latency unit conversion is centralized in secondsToMillisInt(sec float64) using math.Round(sec*1000)"
  - "Query API opens a dedicated SQLite read connection via metricspersist.InitDB(path) (no writer DB reuse)"

patterns-established:
  - "Management metrics contract: GET /v0/management/metrics supports request_id lookup and mode-based queries"
  - "Deterministic time defaults: when from/to omitted, effective range is [now-1h, now] in UTC and echoed in meta"

# Metrics
duration: 7min
completed: 2026-01-30
---

# Phase 4 Plan 2: Management Metrics Query Summary

**在 management namespace 上线统一查询入口 `GET /v0/management/metrics`，支持 request_id 单条查询（含 ttft/tpot 秒->毫秒取整）并锁定输入校验与 meta 回显契约**

## Performance

- **Duration:** 6m 54s
- **Started:** 2026-01-30T09:05:11Z
- **Completed:** 2026-01-30T09:12:05Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments
- 统一入口（`internal/api/server.go`）注册 `GET /v0/management/metrics`，复用既有 management middleware。
- handler（`internal/api/handlers/management/metrics.go`）实现 request_id 精确查询、fail-fast 校验与 meta envelope。
- 单测（`test/metrics_management_test.go`）覆盖 request_id 成功/404、mode/from/to/streaming 校验，以及默认最近 1h 的 meta 回显（now 注入固定）。

## Task Commits

每个任务均以原子提交落地：

1. **Task 1: 注册 management 路由 /v0/management/metrics** - `bb11f68` (feat)
2. **Task 2: 实现 GetMetrics（request_id 查询 + 参数校验 + meta 回显）** - `c1bd296` (feat)
3. **Task 3: 增加基础契约测试（request_id + 参数校验）** - `f311555` (test)

## Files Created/Modified
- `internal/api/server.go` - 在 management group 注册 `GET /metrics` 路由。
- `internal/api/handlers/management/handler.go` - 增加 nowUTC 注入与 metrics 读库 lazy init（避免复用 writer DB）。
- `internal/api/handlers/management/metrics.go` - `GetMetrics` 入口：request_id 分支可用，mode 分支完成校验 + meta 并返回 501 占位。
- `internal/api/handlers/management/metrics_units.go` - `secondsToMillisInt` 统一取整策略（math.Round）。
- `test/metrics_management_test.go` - management metrics contract tests。

## Decisions Made
- 将 Query API 的错误响应也统一为包含 `error` 字段的 JSON，并在可用时仍回显 `meta`（确保 501 分支也能锁定 meta contract）。
- request_id 分支忽略 mode/from/to/bucket，但仍对 provider/model/streaming 做共享的 fail-fast 校验与 meta 回显。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 调整任务执行顺序以满足编译与原子提交约束**
- **Found during:** Task 1（路由引用 `GetMetrics` 需要方法已存在，但 Task 1 只允许改 `internal/api/server.go`）
- **Issue:** 如果严格先做 Task 1，会导致 `go test ./...` 在 Task 1 结束时无法通过；同时会迫使 Task 1 commit 夹带 handler 代码。
- **Fix:** 先完成 Task 2 以提供 `GetMetrics` 方法，再完成 Task 1 仅提交路由注册。
- **Verification:** `go test ./...`
- **Committed in:** `c1bd296` + `bb11f68`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** 仅调整执行顺序，未改变对外契约与交付范围。

## Issues Encountered
- `.planning/STATE.md` 已按执行协议更新，但根据本次要求不纳入 docs commit（避免提交与本 plan 无关的 planning docs）。若因此导致工作区不干净，请由 orchestrator 决定是否提交/回滚该变更。
- docs commit 需要仅 stage `04-02-PLAN.md` + `04-02-SUMMARY.md`：其中 `04-02-PLAN.md` 本次未发生内容变更，因此不会出现在 commit diff 中（但 staging 操作会保持在允许范围内）。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- handler 已锁定查询入口 contract 与 meta 计算规则，后续 04-03/04-04 只需填充 percentiles/buckets 聚合输出。
- `secondsToMillisInt` 已作为单一单位转换入口，可直接复用到 percentiles/buckets 输出路径。

---
*Phase: 04-query-api*
*Completed: 2026-01-30*
