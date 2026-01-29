---
milestone: v1
audited: 2026-01-30T09:56:18Z
status: gaps_found
scores:
  requirements: 10/10
  phases: 4/4
  integration: 3/4
  flows: 2/3
gaps:
  requirements: []
  integration:
    - "Streaming terminal errors are not recorded as failures (OpenAI/Gemini): RequestState.LastError is never set and HTTP status may remain 200, so Query API success/failure split can be wrong."
    - "Some providers may not publish a usage record when tokens are zero (non-failed), which can prevent persistence entirely (no usage -> MetricsPlugin not invoked -> no DB row)."
  flows:
    - "Streaming failure path: an upstream streaming error can produce an error SSE payload but still be persisted/classified as success."
tech_debt:
  - phase: 02-metrics-collection
    items:
      - "Phase 2 verification text still references JSONL writer, but Phase 3 disabled JSONL in favor of SQLite. Update docs/report/comments to avoid operator confusion."
      - "Performance impact is only design-validated; consider running a small load test to confirm no noticeable streaming cadence regression."
  - phase: 03-persistence
    items:
      - "Docs drift: `重要命令.txt` references JSONL metrics logs; code now persists to `logs/metrics.db`."
      - "Persistence is best-effort (drops on queue full, suppresses insert errors) to protect latency; document expected loss behavior under overload."
  - phase: 04-query-api
    items:
      - "`created_at` is returned as raw SQLite `CURRENT_TIMESTAMP` string for request_id queries; consider clarifying/normalizing format."
      - "Percentiles time filtering relies on `created_at >= datetime(?)`; if `created_at` storage format changes, filtering semantics must be updated (buckets already use `unixepoch(...)`)."
---

# Milestone v1 Audit (TPS Metrics)

## Definition Of Done (Milestone Scope)

Source documents:
- Intent: `.planning/PROJECT.md`
- Requirements: `.planning/REQUIREMENTS.md`
- Plan: `.planning/ROADMAP.md`

Milestone v1 is considered done when:
1) v1 requirements (METR-01..04, DISP-01..02, STOR-01..04) are satisfied.
2) All planned phases are verified (have `*-VERIFICATION.md`, status passed).
3) Cross-phase integration is correct (request -> metrics -> persistence -> query API), including failure semantics.
4) E2E flows work for the primary user journeys.

## Phase Verification Aggregation

| Phase | Status | Verified | Notes |
|------|--------|----------|-------|
| 01-metrics-foundation | passed | 2026-01-29 | Core TPS/TTFT/TPOT calculations + provider/model/streaming grouping |
| 02-metrics-collection | passed | 2026-01-29 | Streaming TTFT sampling unified via PrefetchedChunk; human verification recommended for runtime behavior |
| 03-persistence | passed | 2026-01-30 | SQLite persistence, migrations, retention, async writer; JSONL legacy disabled |
| 04-query-api | passed | 2026-01-30 | `GET /v0/management/metrics` (request_id/percentiles/buckets) + contract tests |

## Requirements Coverage (v1)

All v1 requirements are covered by phase verification evidence (even though some checkboxes in `.planning/REQUIREMENTS.md` remain unchecked).

| Requirement | Phase | Status | Evidence |
|------------|-------|--------|----------|
| METR-01 | 01 | satisfied | `.planning/phases/01-metrics-foundation/01-VERIFICATION.md` |
| METR-02 | 01 | satisfied | `.planning/phases/01-metrics-foundation/01-VERIFICATION.md` |
| METR-03 | 01 | satisfied | `.planning/phases/01-metrics-foundation/01-VERIFICATION.md` |
| METR-04 | 01 | satisfied | `.planning/phases/01-metrics-foundation/01-VERIFICATION.md` |
| DISP-01 | 02 | satisfied | `.planning/phases/02-metrics-collection/02-VERIFICATION.md` |
| DISP-02 | 02 | satisfied | `.planning/phases/02-metrics-collection/02-VERIFICATION.md` |
| STOR-01 | 03 | satisfied | `.planning/phases/03-persistence/03-VERIFICATION.md` |
| STOR-02 | 04 | satisfied | `.planning/phases/04-query-api/04-VERIFICATION.md` |
| STOR-03 | 04 | satisfied | `.planning/phases/04-query-api/04-VERIFICATION.md` |
| STOR-04 | 04 | satisfied | `.planning/phases/04-query-api/04-VERIFICATION.md` |

## Cross-Phase Integration Check

Integration review focused on the end-to-end chain:
"AI request" -> RequestState/TTFT sampling -> usage publish -> MetricsPlugin -> SQLite -> management query API.

### Working Links (Confirmed)

- RequestState is attached before writing streaming chunks in OpenAI/Gemini/Claude handlers.
- TTFT sampling is unified via `PrefetchedChunk` and `ForwardStream` calling `metricsruntime.MaybeRecordFirstContentToken` after flush.
- MetricsPlugin enqueues MetricRecord into async SQLite writer (`logs/metrics.db`).
- Management endpoint `GET /v0/management/metrics` reads the same DB and supports request_id, percentiles, and buckets.

### Critical Integration Gaps (Blockers)

1) Streaming failure semantics are not reliably persisted/classified:
   - OpenAI streaming terminal errors write an SSE error payload but do not set HTTP status (e.g. `sdk/api/handlers/openai/openai_handlers.go` WriteTerminalError closure).
   - Gemini streaming terminal errors likewise do not set HTTP status (`sdk/api/handlers/gemini/gemini_handlers.go`).
   - `RequestState.SetLastError(...)` exists but is not used by these terminal error paths, so DB `error_info` can remain empty.
   - Query API uses `status_code`/`error_info` to split success vs failure, so real streaming failures can be miscounted.

2) Persistence depends on a usage record being published:
   - `publishWithOutcome(...)` can skip publishing when tokens are all zero and the request is not marked failed.
   - If a provider path can end without usage metadata (or without an ensure-publish fallback), the request may produce no DB row.

## End-to-End Flows

### Flow 1: Streaming success -> summary -> persistence -> query

- Expected: streaming request yields TTFT/TPS/TPOT, prints a single summary, persists a row, and can be queried via percentiles/buckets.
- Status: mostly complete, but still needs runtime human verification (see Phase 2 notes) to confirm summary behavior and persistence.

### Flow 2: Streaming failure -> persistence -> query failure group

- Expected: if streaming fails mid-flight, DB row should reflect failure (status or error_info) so Query API failure group is accurate.
- Status: broken (see Critical Integration Gap #1).

### Flow 3: Management query API security boundary

- Expected: management metrics endpoint is protected by management middleware and not exposed without auth.
- Status: wired, but review CORS posture for management endpoints as security hardening tech debt.

## Deferred / Tech Debt Summary

Top items to track (non-blocking):
- Documentation drift around JSONL vs SQLite (`重要命令.txt`, Phase 2 wording, and comments).
- Explicitly document best-effort persistence semantics and possible drops under overload.
- Clarify/normalize timestamps returned by request_id query (`created_at`).
