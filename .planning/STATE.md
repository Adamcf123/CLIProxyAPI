# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2025-01-29)

**Core value:** 实时可见的 API 响应性能 — 用户能够获得 TPS 指标汇总并查询历史性能数据
**Current focus:** Persistence

## Current Position

Phase: 3 of 4 (Persistence)
Plan: 2 of 3 in current phase
Status: In progress
Last activity: 2026-01-30 — Completed 03-02-PLAN.md (Async SQLite writer + MetricsPlugin integration)

Progress: [███████████░] 91% (10/11 plans)

## Performance Metrics

**Velocity:**
- Total plans completed: 10
- Average duration: 7 min
- Total execution time: 1.2 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-metrics-foundation | 4 | 4 | 4 min |
| 02-metrics-collection | 4 | 4 | 11 min |
| 03-persistence | 2 | 3 | 5 min |

**Recent Trend:**
- Last 5 plans: 19 min, 12 min, 6 min, 5 min, 5 min
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
- Structured metrics logging writes to logs/metrics-YYYY-MM-DD.jsonl with daily rotation
- MetricsPlugin computes TPS/TTFT/TPOT from usage records and persists to JSONL asynchronously
- Log enqueue is non-blocking; queue-full drops line to ensure zero impact on request path
- Unified TTFT sampling: all streaming providers (OpenAI/Gemini/Claude) now use ForwardStream.PrefetchedChunk to ensure first payload chunk triggers TTFT
- RequestState must be attached BEFORE any write/flush to capture true first token time
- Prefetched chunk pattern: handler peeks first chunk for error detection, then passes to ForwardStream for unified output + metrics

**From Phase 03-persistence:**
- Startup runs embedded SQLite migrations (goose); failures must fail-fast (os.Exit(1))
- SQLite InitDB uses modernc.org/sqlite with WAL/NORMAL/busy_timeout PRAGMAs and a single underlying connection for consistency
- SQLite metrics persistence uses a non-blocking enqueue + background writer; queue-full drops to avoid request latency impact
- SQLite metrics rows are keyed by request_id for de-duplication; request_path is excluded from DB

### Pending Todos

[From .planning/todos/pending/ — ideas captured during sessions]

Count: 1
- .planning/todos/pending/2026-01-30-support-streaming-usage-tokens-gpt-5-2.md

### Blockers/Concerns

[Issues that affect future work]

None yet.

## Session Continuity

Last session: 2026-01-30 07:30Z
Stopped at: Completed 03-02-PLAN.md (Async SQLite writer + MetricsPlugin integration)
Resume file: None
