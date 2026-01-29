# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2025-01-29)

**Core value:** 实时可见的 API 响应性能 — 用户能够获得 TPS 指标汇总并查询历史性能数据
**Current focus:** Metrics Collection

## Current Position

Phase: 2 of 4 (Metrics Collection)
Plan: 1 of 3 in current phase
Status: In progress
Last activity: 2026-01-29 — Completed 02-01-PLAN.md (collection foundation)

Progress: [████████░░░░] 41%

## Performance Metrics

**Velocity:**
- Total plans completed: 5
- Average duration: 6 min
- Total execution time: 0.5 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-metrics-foundation | 4 | 4 | 4 min |
| 02-metrics-collection | 1 | 3 | 12 min |

**Recent Trend:**
- Last 5 plans: 4 min, 4 min, 4 min, 4 min, 12 min
- Trend: ↑ (Phase 2 plans are taking longer)

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

**From Phase 01-metrics-foundation:**
- Fixed 100-request sliding window size for each provider/model combination
- Separate aggregation for streaming vs non-streaming requests
- Extracted SlidingWindow into standalone module with public API (Add, GetAll, Len, GetStats, RestoreFrom)
- Used RWMutex for thread-safe concurrent access to sliding window
- Implemented percentile calculation with linear interpolation for p95/p99
- TPSCollector uses calculator.go functions for all metric calculations (TTFT, TPS, TPOT)
- Non-streaming TTFT calculated as total response time (EndTime - StartTime)
- CompleteRequest returns error for invalid requests instead of silently discarding

**From Phase 02-metrics-collection:**
- TTFT anchor is recorded at the first flushed non-keep-alive payload chunk in ForwardStream (not at keep-alive flush)
- TPSCollector protects windows map with RWMutex; SlidingWindow retains its own internal locking

### Pending Todos

[From .planning/todos/pending/ — ideas captured during sessions]

None yet.

### Blockers/Concerns

[Issues that affect future work]

None yet.

## Session Continuity

Last session: 2026-01-29 16:25Z
Stopped at: Completed 02-01-PLAN.md
Resume file: None
