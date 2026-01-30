---
phase: 09-cancel-disconnect-semantics
plan: 01
subsystem: api
tags: [go, gin, sqlite, metrics, streaming, cancel, disconnect, 499]

# Dependency graph
requires:
  - phase: 08-persistence-contract-observability
    provides: best-effort persistence contract + drop observability
provides:
  - status_code=499 作为 canceled 的持久化信号（error_info 始终为空）
  - RequestState 内固化 failure > canceled > success 的不可覆盖写入优先级
  - streaming + response writer 两条路径统一检测 cancel/disconnect 并写入 RequestState
affects: [09-02-query-api, percentiles, buckets, canceled_count]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "RequestState outcome 写入以优先级为单一真相（failure > canceled > success）"
    - "canceled 持久化只用 status_code=499，不复用 error_info"

key-files:
  created: []
  modified:
    - internal/metricsruntime/request_state.go
    - internal/metricsruntime/usage_plugin.go
    - sdk/api/handlers/stream_forwarder.go
    - sdk/api/handlers/stream_forwarder_test.go
    - internal/api/middleware/response_writer.go

key-decisions:
  - "选择方案 A：用 status_code=499 表达 canceled（hard cutover，无 schema 变更）"

patterns-established:
  - "canceled 的落库信号由 usage_plugin 从 RequestState 统一派生，避免 handler 分散写入"

# Metrics
duration: 5 min
completed: 2026-01-30
---

# Phase 09 Plan 01: Cancel/Disconnect Semantics Summary

**Canceled 请求以 status_code=499 持久化，并在写入侧固化 failure > canceled > success 的单一真相，避免被误判为 success。**

## Performance

- **Duration:** 5 min
- **Started:** 2026-01-30T19:56:23Z
- **Completed:** 2026-01-30T20:02:09Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- canceled 的持久化表达（`internal/metricsruntime/request_state.go`）固化为 `status_code=499`，且不会被后续 2xx 覆盖
- canceled 落库（`internal/metricsruntime/usage_plugin.go`）保持 `ErrorInfo=nil`，并跳过 TPSCollector 聚合以避免污染
- cancel/disconnect 检测（`sdk/api/handlers/stream_forwarder.go`、`internal/api/middleware/response_writer.go`）统一写入 RequestState，并对 timeout/upstream error 保持 failure 优先

## Task Commits

Each task was committed atomically:

1. **Task 1: 选择并固化 canceled 的持久化表达（方案 A: status_code=499）** - `2f4563a` (feat)
2. **Task 2: 覆盖 streaming / non-streaming 两条 cancel/disconnect 检测路径并写入 RequestState** - `e8e526b` (feat)

**Plan metadata:** (docs commit created after SUMMARY)

## Files Created/Modified

- `internal/metricsruntime/request_state.go` - 定义 499 语义与写入优先级（failure > canceled > success）
- `internal/metricsruntime/usage_plugin.go` - canceled 以 499 + nil error_info 落库，并跳过 TPSCollector
- `sdk/api/handlers/stream_forwarder.go` - streaming ctx.Done 分支区分 canceled vs DeadlineExceeded，并遵守上游 error 优先
- `sdk/api/handlers/stream_forwarder_test.go` - 锁定 canceled/timeout/upstream error 的不可变语义
- `internal/api/middleware/response_writer.go` - Write/WriteString 记录 write error，并在 Finalize 做最终 canceled 补正

## Decisions Made

- 选择方案 A：用 `status_code=499` 表达 canceled（无 schema 变更，便于 hard cutover），并明确 canceled 不通过 `LastError`/`error_info` 表达。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] 为 RequestState 的 canceled 不可覆盖语义补充单测**

- **Found during:** Task 1 (选择并固化 canceled 的持久化表达)
- **Issue:** Plan 未明确要求单测锁定 "499 不被 2xx 覆盖" 与 "failure 覆盖 canceled" 的写入优先级，存在回归风险
- **Fix:** 在 `internal/metricsruntime/request_state_test.go` 增加覆盖 canceled stickiness 与 failure override 的测试
- **Files modified:** internal/metricsruntime/request_state_test.go
- **Verification:** `go test ./internal/metricsruntime`
- **Committed in:** 2f4563a

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** 仅增加回归锁定，不改变对外能力与契约。

## Issues Encountered

- Task 2 的 plan verify 正则里 `Request()...` 与实际代码 `c.Request...` 不匹配；执行时使用了等价的 `Context\\(\\)\\.Done\\(\\)` 搜索以完成验证。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 写入侧已能稳定把 cancel/disconnect 作为 499 落库且不污染聚合窗口
- 下一步可进入 09-02：Query API 侧引入 canceled 三分法（percentiles 排除 canceled、buckets 单列 canceled_count）并补齐契约测试

---
*Phase: 09-cancel-disconnect-semantics*
*Completed: 2026-01-30*
