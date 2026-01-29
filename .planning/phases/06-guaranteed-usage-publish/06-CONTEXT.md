# Phase 6: Guaranteed Usage Publish - Context

**Gathered:** 2026-01-30
**Status:** Ready for planning

<domain>
## Phase Boundary

When upstream does not return any usage/tokens, the system still guarantees at least one usage record is published so SQLite has a queryable row for the request.

This phase also locks that failure semantics are preserved: failure paths publish a failure record and `ensurePublished` must not override or “steal” failure.

</domain>

<decisions>
## Implementation Decisions

### No-usage persistence semantics (field meaning)
- When usage/tokens are missing, persist token-related fields as `NULL` (meaning “missing/unknown”), not `0`.
- Any derived metrics that depend on tokens (e.g., TPS/TPOT) are `NULL` when tokens are missing.
- TTFT is persisted when it is available from observed streaming behavior; otherwise TTFT is `NULL`.
- “Queryable row” means: beyond `request_id`, persist as much metadata as possible (provider/model/streaming/status_code/timestamps, etc.) so the request is auditable.

### ensurePublished precedence vs failure
- SQLite should contain at most 1 row per `request_id` (stable query semantics).
- `ensurePublished` is a no-op if any record was already published for the request.
- If a failure record was published, `ensurePublished` must not override it (failure record remains authoritative).
- If a request ends successfully but has no usage/tokens and no error signal, it is still classified as success (tokens/derived metrics remain `NULL`).

### Query + aggregation semantics
- Request-level lookup by `request_id` returns the row even when tokens/derived metrics are `NULL`.
- Aggregate queries (e.g., buckets) count these rows in request counts; numeric statistics naturally exclude `NULL` values.
- Percentiles exclude `NULL` metric values (do not treat them as 0).
- No additional explicit “missing usage” flag is required in the Query API; `NULL` is the signal.

### User-visible output (stderr/logs)
- Do not add any new user-facing warning/notice when usage is missing; keep output behavior stable.
- Do not add new structured log fields solely to mark missing usage; `NULL` token/metric fields remain the indicator.
- Missing usage does not change CLI exit codes.

### Claude's Discretion
- None — decisions above are locked.

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 06-guaranteed-usage-publish*
*Context gathered: 2026-01-30*
