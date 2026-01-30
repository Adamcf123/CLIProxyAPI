# Phase 3: Persistence - Context

**Gathered:** 2026-01-30
**Status:** Ready for planning

<domain>
## Phase Boundary

将每次请求产生的指标数据持久化到 SQLite（`logs/metrics.db`），并保证数据库初始化与 schema 迁移正确、写入不影响请求性能。历史查询与聚合 API 属于 Phase 4，不在本阶段范围内。

</domain>

<decisions>
## Implementation Decisions

### Storage shape (row granularity + de-dup)
- Each request produces exactly one DB row (request-level record), not time-window aggregates.
- De-duplication key is `request_id`.

### Write timing (streaming consistency)
- Streaming requests write to DB once, at end-of-stream (no incremental updates).
- If some fields/metrics are unavailable (e.g., tokens missing), still write the row and keep those fields as `null`.

### DB file location
- SQLite file path is `logs/metrics.db`.

### Migration policy
- Auto-migrate schema on startup.
- If migration fails, fail-fast and exit (do not continue with persistence disabled).

### Multi-instance behavior
- Assumption: no concurrent multiple instances writing at the same time.

### Sensitive fields boundary
- Store `request_id`.
- Do NOT store request `path`.

### Retention policy
- Keep most recent 7 days of data.

### JSONL policy (Phase 2 legacy)
- Stop writing JSONL metrics logs after DB persistence is implemented; DB becomes the single source of truth.

### Claude's Discretion
- Exact schema column set for metrics fields (as long as it can represent all required fields from Phase 3 success criteria).
- The concrete cleanup mechanism for “keep 7 days” (e.g., periodic deletion strategy) as long as it stays within Phase 3 boundary.

</decisions>

<specifics>
## Specific Ideas

- No additional preferences provided.

</specifics>

<deferred>
## Deferred Ideas

- None — discussion stayed within phase scope.

</deferred>

---

*Phase: 03-persistence*
*Context gathered: 2026-01-30*
