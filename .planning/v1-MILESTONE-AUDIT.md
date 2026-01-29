---
milestone: v1
audited: 2026-01-31T20:33:13Z
status: gaps_found
scores:
  requirements: 10/10
  phases: 10/11
  integration: 1/1
  flows: 5/5
gaps:
  requirements: []
  integration: []
  flows: []
  phases:
    - "Phase 09 is missing a required verification artifact: .planning/phases/09-cancel-disconnect-semantics/09-VERIFICATION.md"
tech_debt:
  - phase: 02-metrics-collection
    items:
      - "Performance impact is design-verified + Phase 11 runtime-validated, but there is no automated SLO/latency regression test to prevent future drift."
  - phase: 04-query-api
    items:
      - "request_id branch returns `created_at` as a raw SQLite timestamp string; consider documenting or normalizing to RFC3339 to reduce client parsing ambiguity."
  - phase: 08-persistence-contract-observability
    items:
      - "Writer goroutine exits on Prepare failure; it becomes observable (degraded) but does not self-restart. Confirm desired long-lived behavior (ops policy)."
  - phase: 10-request-id-robustness
    items:
      - "If crypto/rand fails in GenerateRequestID(), it returns all-zero ID. Collision becomes observable (degraded) but the event is still severe; consider whether fail-fast is preferable."
  - phase: milestone-integration
    items:
      - "Provider/model semantics may differ between stderr metrics_summary (RequestState) and SQLite/Query API (usage record). Clarify and lock the semantic contract (or output both explicitly)."
      - "Phase 11 runtime artifacts do not include a saved management request_id query response; add an artifact that proves `GET /v0/management/metrics?request_id=...` works in a real run without leaking secrets."
---

# Milestone v1 Audit (TPS Metrics)

## Definition Of Done (Milestone Scope)

Source documents:
- Intent: `.planning/PROJECT.md`
- Requirements: `.planning/REQUIREMENTS.md`
- Plan: `.planning/ROADMAP.md`

Milestone v1 is considered done when:
1) v1 requirements (METR-01..04, DISP-01..02, STOR-01..04) are satisfied.
2) All milestone phases in `.planning/ROADMAP.md` are verified (have `*-VERIFICATION.md`, status passed).
3) Cross-phase integration is correct (request -> metrics -> persistence -> query API), including failure semantics.
4) E2E flows work for the primary user journeys.

## Milestone Scope

This audit treats milestone v1 scope as the full set of phases marked complete in `.planning/ROADMAP.md`:

- Phase 1..4: Core delivery for v1 requirements (metrics + display + persistence + query)
- Phase 5..11: Hardening, semantic gap closure, and runtime validation (executed and considered part of current milestone delivery)

## Phase Verification Aggregation

| Phase | Status | Verified | Notes |
|------|--------|----------|-------|
| 01-metrics-foundation | passed | 2026-01-29 | METR-01..04 are verified in code + tests |
| 02-metrics-collection | passed | 2026-01-29 | TTFT sampling unified via PrefetchedChunk; runtime behavior validated in Phase 11 |
| 03-persistence | passed | 2026-01-30 | SQLite `logs/metrics.db` persistence + migrations + retention; JSONL removed |
| 04-query-api | passed | 2026-01-30 | `GET /v0/management/metrics` (request_id/percentiles/buckets) + contract tests |
| 05-streaming-failure-semantics | passed | 2026-01-30 | Terminal errors persisted as failure (LastError -> error_info) and Query API classification fixed |
| 06-guaranteed-usage-publish | passed | 2026-01-30 | No-usage still publishes 1 usage record; ensures a DB row exists |
| 07-docs-traceability-cleanup | passed | 2026-01-30 | Docs/traceability aligned to SQLite as single source of truth |
| 08-persistence-contract-observability | passed | 2026-01-30 | Best-effort persistence contract + meta.persistence only when degraded |
| 09-cancel-disconnect-semantics | UNVERIFIED | N/A | Missing `.planning/phases/09-cancel-disconnect-semantics/09-VERIFICATION.md` (blocker) |
| 10-request-id-robustness | passed | 2026-01-31 | 64-bit request_id + collision observability (request_id_conflict) |
| 11-runtime-validation | passed | 2026-01-31 | Runtime validation + secrets guard; evidence artifacts in `.planning/phases/11-runtime-validation/artifacts/` |

## Requirements Coverage (v1)

All v1 requirements are covered by phase verification evidence.

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

Notes:
- Phase 5/6/8/9/10/11 strengthen v1 correctness and operability (streaming failure semantics, no-usage persistence, persistence observability, cancel/disconnect semantics, request_id robustness, and runtime validation).

## Cross-Phase Integration Check

Integration review focused on the end-to-end chain:
"AI request" -> RequestState/TTFT sampling -> usage publish -> MetricsPlugin -> SQLite -> management query API.

### Working Links (Confirmed)

- RequestState is attached before writing streaming chunks in OpenAI/Gemini/Claude handlers.
- TTFT sampling is unified via `PrefetchedChunk` and `ForwardStream` calling metricsruntime sampling after flush.
- Streaming terminal errors persist failure signals (`LastError` -> `error_info`) and Query API classification is locked by tests.
- MetricsPlugin enqueues MetricRecord into async SQLite writer (`logs/metrics.db`).
- Management endpoint `GET /v0/management/metrics` reads the same DB and supports request_id, percentiles, and buckets.

### Critical Integration Gaps (Blockers)

None found in the code/test chain.

Milestone remains `gaps_found` solely due to a missing verification artifact for Phase 09.

## End-to-End Flows

### Flow 1: Streaming success -> summary -> persistence -> query

- Expected: streaming request yields TTFT/TPS/TPOT, prints a single summary, persists a row, and can be queried via request_id/percentiles/buckets.
- Status: verified via Phase 4/5/6 contract tests + Phase 11 runtime artifacts.

### Flow 2: Streaming failure -> persistence -> query failure group

- Expected: if streaming fails mid-flight, DB row should reflect failure (status or error_info) so Query API failure group is accurate.
- Status: verified via Phase 5 tests + Phase 11 runtime artifacts.

### Flow 3: Client cancel/disconnect -> persisted as canceled (499) -> excluded from success/failure aggregation

- Expected: client cancellation/disconnect is recorded as canceled (499 with nil error_info) and does not pollute success/failure percentiles/buckets.
- Status: tests exist and Phase 11 runtime artifacts include cancel scenario; Phase 09 still needs a formal verification file.

### Flow 4: No-usage -> still persisted/queryable

- Expected: a request that ends without usage metadata still produces a queryable row (tokens/metrics may be NULL).
- Status: verified via Phase 6 tests + Phase 11 runtime artifacts.

### Flow 5: Persistence degraded -> meta.persistence only when degraded

- Expected: `meta.persistence` is omitted when healthy and emitted with minimal safe fields only when degraded.
- Status: verified via Phase 8 contract tests + Phase 11 degraded artifact.

### Flow 6: Management query API security boundary

- Expected: management metrics endpoint is protected by management middleware and not exposed without auth.
- Status: wired (not re-audited in depth here); keep least-privilege and CORS posture review as routine hardening.

## Deferred / Tech Debt Summary

Top items to track (non-blocking):
- Clarify the semantic contract for provider/model across stderr metrics_summary vs SQLite/Query API.
- Consider adding a runtime artifact that captures a real `GET /v0/management/metrics?request_id=...` response (Phase 11 scripts).
- Clarify/normalize timestamps returned by request_id query (`created_at`).
