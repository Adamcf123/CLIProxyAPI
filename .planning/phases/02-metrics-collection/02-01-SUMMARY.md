---
phase: 02-metrics-collection
plan: 01
subsystem: api
tags: [go, gin, streaming, sse, metrics, ttft, concurrency]

# Dependency graph
requires:
  - phase: 01-metrics-foundation
    provides: TPSCollector + SlidingWindow metric aggregation primitives
provides:
  - Concurrency-safe TPSCollector windows map access (no map races/panic)
  - Per-request RequestState bound to gin.Context for metrics sampling points
  - ForwardStream TTFT anchor sampling after first flushed payload chunk (excluding keep-alive)
affects: [02-metrics-collection, logging, realtime-display, persistence]

# Tech tracking
tech-stack:
  added: []
  patterns: ["Per-request RequestState stored in gin.Context", "TTFT sampling at stream flush boundary"]

key-files:
  created:
    - internal/metricsruntime/request_state.go
  modified:
    - internal/metrics/collector.go
    - sdk/api/handlers/stream_forwarder.go

key-decisions:
  - "TTFT anchor is recorded at the first flushed non-keep-alive payload chunk in ForwardStream (not at keep-alive flush)"
  - "TPSCollector protects windows map with RWMutex; SlidingWindow retains its own internal locking"

patterns-established:
  - "Single TTFT sampling point: ForwardStream flush hook"
  - "Request-scoped state typed API: AttachRequestState/GetRequestState/MaybeRecordFirstToken"

# Metrics
duration: 11m31s
completed: 2026-01-29
---

# Phase 02 Plan 01: Metrics Collection Foundation Summary

**ForwardStream 在首个有效 chunk flush 后采集 TTFT 锚点，并让 TPSCollector 的 windows map 在并发场景下设计上不可能 data race。**

## Performance

- **Duration:** 11m31s
- **Started:** 2026-01-29T16:14:14Z
- **Completed:** 2026-01-29T16:25:45Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- 指标采集的并发基础（`internal/metrics/collector.go`）加固：windows map 访问加锁，避免高并发下 map 读写崩溃
- 单请求指标状态对象（`internal/metricsruntime/request_state.go`）建立：为跨采样点汇聚字段提供统一载体
- TTFT 采样点统一（`sdk/api/handlers/stream_forwarder.go:ForwardStream`）：只在首个有效 payload chunk flush 后记录首 token 时间，排除 keep-alive

## Task Commits

Each task was committed atomically:

1. **Task 1: 让 TPSCollector 的 windows map 并发安全** - `5d7f2b8` (fix)
2. **Task 2: 建立单请求指标状态对象并挂钩 ForwardStream 的首 token 采样** - `99cc301` (feat)

## Files Created/Modified
- `internal/metrics/collector.go` - TPSCollector 增加 RWMutex，保护 windows map 的读写并实现并发安全的 getOrCreateWindow
- `internal/metricsruntime/request_state.go` - RequestState + gin.Context 绑定/读取 + 首 token 采样 helper（过滤 keep-alive/空 chunk）
- `sdk/api/handlers/stream_forwarder.go` - ForwardStream 在写出并 flush 后调用 MaybeRecordFirstToken

## Decisions Made
- 以“写出并 flush 后的首个有效 payload chunk”为 TTFT 锚点采样点，避免 keep-alive 造成 TTFT 偏小
- TPSCollector 仅对 windows map 加锁；窗口内部仍由 SlidingWindow 自身锁语义保证并发安全

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- 采集基础已就位：后续可以在 usage 插件/请求结束路径中读取 RequestState 并调用 TPSCollector.CompleteRequest 汇总
- ForwardStream 的 TTFT hook 已统一：各 provider handler 无需重复首 token 采样逻辑

---
*Phase: 02-metrics-collection*
*Completed: 2026-01-29*
