# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2025-01-29)

**Core value:** 实时可见的 API 响应性能 — 用户能够获得 TPS 指标汇总并查询历史性能数据
**Current focus:** Phase 11 complete (runtime validation verified)

## Current Position

Phase: 11 of 11 (Runtime Validation)
Plan: 4 of 4 in current phase
Status: Phase complete (verified)
Last activity: 2026-02-01 — Quick task 001: remove non-TTY live metrics printing (.planning/quick/001-remove-live-metrics-printing/001-SUMMARY.md)

Progress: [██████████] 100% of planned plans to date (39/39)

## Performance Metrics

**Velocity:**
- Total plans completed: 39
- Average duration: 7 min
- Total execution time: 2.3 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01-metrics-foundation | 4 | 4 | 4 min |
| 02-metrics-collection | 4 | 4 | 11 min |
| 03-persistence | 3 | 3 | 5 min |
| 04-query-api | 4 | 4 | 7 min |
| 05-streaming-failure-semantics | 3 | 3 | 4 min |
| 06-guaranteed-usage-publish | 5 | 5 | 2 min |
| 07-docs-traceability-cleanup | 4 | 4 | 2 min |
| 08-persistence-contract-observability | 2 | 2 | 5 min |
| 09-cancel-disconnect-semantics | 3 | 3 | 5 min |

**Recent Trend:**
- Last 5 plans: 1 min, 2 min, 1 min, 4 min, 5 min
- Trend: → (stable)

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

**From Phase 08-persistence-contract-observability:**
- Best-effort persistence drops are observable via process-lifetime health (dropped_total + last_drop + quiet-period degraded)
- Drop reasons exposed as stable enum codes: queue_full, writer_not_started, insert_failure
- /v0/management/metrics emits meta.persistence only when degraded to preserve default JSON shape

**From Phase 09-cancel-disconnect-semantics:**
- Outcome 三分法：success / failure / canceled，canceled 用 status_code=499 表达
- Priority: failure > canceled > success — failure 可以覆盖 canceled，但 success 不能
- Timeout (DeadlineExceeded) 归类为 failure (504)，不是 canceled
- Canceled 的 ErrorInfo 必须为空，且不计入 TPS/TPOT 聚合
- Query API percentiles 排除 canceled 样本，buckets 每 bucket 单列 canceled_count

**From Phase 11-runtime-validation:**
- stderr `metrics_summary.tracking_id` 与 SQLite `metrics.request_id` 对齐，保证运行时证据可对照
- OpenAI-compat 流式在缺失 `[DONE]` 的 EOF 场景被视为 terminal error（可持久化失败语义）
- Usage publish 在 handler tail 之后时，status_code 通过 Gin writer fallback 确保可落库
- Disk request logs omit sensitive auth headers entirely (Authorization / Proxy-Authorization / X-Management-Key / Cookie)

### Pending Todos

[From .planning/todos/pending/ — ideas captured during sessions]

Count: 0

### Blockers/Concerns

[Issues that affect future work]

- (Closed) Phase 11 secrets gap: secrets guard scans artifacts and no raw auth headers persist on disk (see `.planning/phases/11-runtime-validation/11-VERIFICATION.md`).

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 001 | Remove live metrics printing | 2026-02-01 | 44d4d24 | [001-remove-live-metrics-printing](./quick/001-remove-live-metrics-printing/) |

## Session Continuity

Last session: 2026-02-01 20:10Z
Stopped at: Quick task 001 complete (.planning/quick/001-remove-live-metrics-printing/001-SUMMARY.md)
Resume file: None
