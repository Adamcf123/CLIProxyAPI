# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2025-01-29)

**Core value:** 实时可见的 API 响应性能 — 用户能够获得 TPS 指标汇总并查询历史性能数据
**Current focus:** Milestone complete (ready for audit)

## Current Position

Phase: 6 of 6 (Guaranteed Usage Publish)
Plan: 5 of 5 in current phase
Status: Verified
Last activity: 2026-01-30 — Verified Phase 6 goal (06-VERIFICATION.md)

Progress: [██████████] 100% of planned plans to date (23/23) + Phase 6 verified

## Performance Metrics

**Velocity:**
- Total plans completed: 23
- Average duration: 7 min
- Total execution time: 1.7 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-metrics-foundation | 4 | 4 | 4 min |
| 02-metrics-collection | 4 | 4 | 11 min |
| 03-persistence | 3 | 3 | 5 min |
| 04-query-api | 4 | 4 | 7 min |
| 05-streaming-failure-semantics | 3 | 3 | 4 min |
| 06-guaranteed-usage-publish | 5 | 5 | 2 min |

**Recent Trend:**
- Last 5 plans: 4 min, 12 min, 2 min, 6 min, 1 min
- Trend: ↓ (improving)

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
- Live metrics progress + summary are stderr-only; TTY gates line overwrite (\r + ANSI clear)
- Summary is emitted as a single searchable line (metrics_summary JSON); missing usage keeps tokens/throughput as null
- MetricsPlugin computes TPS/TTFT/TPOT from usage records and persists asynchronously to SQLite (default `logs/metrics.db`)
- Log enqueue is non-blocking; queue-full drops line to ensure zero impact on request path
- Unified TTFT sampling: all streaming providers (OpenAI/Gemini/Claude) now use ForwardStream.PrefetchedChunk to ensure first payload chunk triggers TTFT
- RequestState must be attached BEFORE any write/flush to capture true first token time
- Prefetched chunk pattern: handler peeks first chunk for error detection, then passes to ForwardStream for unified output + metrics

**From Phase 03-persistence:**
- Startup runs embedded SQLite migrations (goose); failures must fail-fast (os.Exit(1))
- SQLite InitDB uses modernc.org/sqlite with WAL/NORMAL/busy_timeout PRAGMAs and a single underlying connection for consistency
- SQLite metrics persistence uses a non-blocking enqueue + background writer; queue-full drops to avoid request latency impact
- SQLite metrics rows are keyed by request_id for de-duplication; request_path is excluded from DB
- Retention enforcement deletes rows older than 7 days after migrations and runs daily in the writer goroutine
- Legacy metrics file logging is decommissioned; SQLite is the single source of truth

**From Phase 04-query-api:**
- SQLite metrics schema includes streaming as a first-class dimension (INTEGER 0/1), with default 0 for historical rows
- Writer treats nil streaming as 0 (fail-closed) to avoid NULL group keys

**From Phase 05-streaming-failure-semantics:**
- Streaming terminal errors persist via RequestState.LastError and map into MetricRecord.ErrorInfo for Query API classification

### Pending Todos

[From .planning/todos/pending/ — ideas captured during sessions]

Count: 1
- .planning/todos/pending/2026-01-30-support-streaming-usage-tokens-gpt-5-2.md

### Blockers/Concerns

[Issues that affect future work]

None yet.

## Session Continuity

Last session: 2026-01-30 15:31Z
Stopped at: Verified Phase 6 goal (06-VERIFICATION.md)
Resume file: None
