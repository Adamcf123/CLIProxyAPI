---
phase: 01-metrics-foundation
plan: 04
subsystem: metrics
tags: [tps, ttft, tpot, sliding-window, percentiles, go, testing]

# Dependency graph
requires:
  - phase: 01-metrics-foundation
    plan: 01
    provides: RequestMetrics, WindowStats, MetricKey types
  - phase: 01-metrics-foundation
    plan: 02
    provides: CalculateTTFT, CalculateTPS, CalculateTPOT, ValidateMetrics functions
  - phase: 01-metrics-foundation
    plan: 03
    provides: SlidingWindow implementation with GetStats
provides:
  - TPSCollector with integrated calculator functions and sliding window aggregation
  - CompleteRequest method using calculator.go functions for metric calculation
  - GetAllKeys method for enumerating metric groups
  - Comprehensive unit tests for TPSCollector
affects: [01-expose-metrics-api, 02-integrate-with-proxy, 03-persistence-layer]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Error propagation: CompleteRequest returns error instead of silently discarding invalid requests
    - Separation of concerns: Calculator functions used for all metric calculations
    - Non-streaming TTFT: Total response time instead of FirstTokenTime - StartTime

key-files:
  created:
    - internal/metrics/collector_test.go
  modified:
    - internal/metrics/collector.go

key-decisions:
  - "Non-streaming TTFT calculated as total response time (EndTime - StartTime)"
  - "CompleteRequest returns error for invalid requests instead of silently discarding"
  - "TPSCollector uses window.go SlidingWindow API instead of duplicating implementation"

patterns-established:
  - "Pattern: Error-first validation in CompleteRequest before any calculation"
  - "Pattern: Streaming mode determines which calculator functions to call"

# Metrics
duration: 5min
completed: 2026-01-29
---

# Phase 01: Metrics Foundation Plan 04 Summary

**TPSCollector with integrated calculator functions, sliding window aggregation, and comprehensive error handling**

## Performance

- **Duration:** 5 min
- **Started:** 2026-01-29T11:46:01Z
- **Completed:** 2026-01-29T11:51:00Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Refactored TPSCollector to use calculator.go functions for all metric calculations
- Added error handling to CompleteRequest (returns error for invalid requests)
- Implemented non-streaming TTFT calculation as total response time
- Added GetAllKeys() method to enumerate all metric groups with data
- Created comprehensive unit tests covering all TPSCollector functionality

## Task Commits

Each task was committed atomically:

1. **Task 1: 完善 TPSCollector 结构体和方法** - `e62fe92` (feat)
2. **Task 2: 添加 TPSCollector 单元测试** - `e62fe92` (feat - combined)

**Plan metadata:** (pending final docs commit)

## Files Created/Modified

- `internal/metrics/collector.go` - Refactored to use calculator functions, added GetAllKeys, error handling
- `internal/metrics/collector_test.go` - 13 comprehensive unit tests for all TPSCollector methods

## Decisions Made

1. **Non-streaming TTFT calculation**: For non-streaming requests, TTFT is calculated as total response time (EndTime - StartTime) instead of requiring FirstTokenTime. This handles the case where non-streaming responses don't have a discrete "first token" event.

2. **Error propagation**: CompleteRequest returns error for invalid requests (nil metrics, zero tokens, invalid timing) instead of silently discarding them. This makes debugging easier and follows fail-loud principles.

3. **Window API usage**: TPSCollector uses window.go's SlidingWindow API (Add, GetAll, GetStats, Len) instead of duplicating implementation or accessing private fields directly.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

1. **Duplicate type definitions**: During initial build, discovered that collector.go and window.go both defined `windowSize` constant and `SlidingWindow` type. This was because 01-03 plan had already created window.go. Fixed by refactoring collector.go to use window.go's SlidingWindow API instead of duplicating the implementation.

2. **Unused variable warning**: `window` variable was declared but not used in StartRequest. Fixed by removing the assignment while keeping the getOrCreateWindow call.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

TPSCollector is now complete and ready for:
- **Plan 01-05**: Expose metrics via HTTP API endpoints
- **Phase 02**: Integration with proxy layer for automatic metric collection

**Blockers/Concerns:**
- window.go (01-03) SUMMARY.md should be created to complete phase 1 documentation
- All three plans (01-01, 01-02, 01-03) should have SUMMARY.md files for complete context

---
*Phase: 01-metrics-foundation*
*Completed: 2026-01-29*
