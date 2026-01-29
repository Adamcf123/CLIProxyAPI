---
phase: 07-docs-traceability-cleanup
plan: 02
subsystem: docs
tags: [metrics, sqlite, management-api]

requires:
  - phase: 03-persistence
    provides: SQLite metrics persistence at logs/metrics.db
  - phase: 04-query-api
    provides: GET /v0/management/metrics query endpoint
provides:
  - README operator quick path for metrics storage + querying
  - Removal of legacy JSONL phrasing in operator-facing docs
affects: [audit, operations, troubleshooting]

tech-stack:
  added: []
  patterns: ["Docs: treat SQLite as metrics single source of truth"]

key-files:
  created:
    - .planning/phases/07-docs-traceability-cleanup/07-02-SUMMARY.md
  modified:
    - internal/usage/metrics_plugin.go
    - README.md
    - README_CN.md

key-decisions:
  - "Keep README Metrics section minimal: document storage path + two query entry points only"

patterns-established:
  - "Operator docs must not describe metrics persistence as JSONL when SQLite is the source of truth"

duration: 2min
completed: 2026-01-30
---

# Phase 7 Plan 02: Docs Traceability Cleanup Summary

**README now documents metrics SQLite storage (logs/metrics.db) and the shortest query path via GET /v0/management/metrics with X-Management-Key**

## Performance

- Duration: 2 min
- Started: 2026-01-30T16:51:33Z
- Completed: 2026-01-30T16:52:52Z

## What Changed

- MetricsPlugin registration comment (`internal/usage/metrics_plugin.go`) removes legacy JSONL phrasing and describes SQLite persistence (`logs/metrics.db`).
- README (`README.md`) and Chinese README (`README_CN.md`) add `## Metrics (TPS/TTFT/TPOT)` documenting:
  - Metrics are persisted to `logs/metrics.db` (SQLite single source of truth)
  - A read-only `sqlite3` query example
  - A Management Query API example: `GET /v0/management/metrics` with `X-Management-Key`

## Why

Operator-facing docs previously contained legacy JSONL wording, which risks misdiagnosing “where metrics live” in real deployments.
This plan aligns the README quick path with the actual implementation (SQLite + Management API), reducing time-to-triage.

## Evidence (Source-of-Truth in Code)

- Metrics DB location is hard-coded and initialized as SQLite in server mode (`cmd/server/main.go`): uses `logs/metrics.db`, runs migrations, and starts the async writer.
- Management route wiring exposes the metrics query endpoint (`internal/api/server.go`): router group `/v0/management` registers `GET /metrics` -> `GET /v0/management/metrics`.
- Management auth requires a key header (`internal/api/handlers/management/handler.go`): accepts `X-Management-Key` (or `Authorization: Bearer <key>`) and denies access without a configured key.

## Verify

Executed during plan:

- Task 1 verify (comment drift check):
  - `python - <<'PY' ... PY` (plan embedded script)
- Task 2 verify (README/README_CN checks + forbid jsonl wording):
  - `python - <<'PY' ... PY` (plan embedded script)
- Task 3 verify (summary content grep):
  - `rg -nS "Phase 7" .planning/phases/07-docs-traceability-cleanup/07-02-SUMMARY.md`
  - `rg -nS "README\\.md" .planning/phases/07-docs-traceability-cleanup/07-02-SUMMARY.md`
  - `rg -nS "internal/usage/metrics_plugin\\.go" .planning/phases/07-docs-traceability-cleanup/07-02-SUMMARY.md`
  - `rg -nS "logs/metrics\\.db|v0/management/metrics" .planning/phases/07-docs-traceability-cleanup/07-02-SUMMARY.md`

## Task Commits

Each task was committed atomically:

1. Task 1: Fix MetricsPlugin JSONL phrasing - `5d50ee6` (docs)
2. Task 2: Document metrics storage + query quick path - `8b9e8c3` (docs)
3. Task 3: Add this audit-ready execution summary - (this commit)

## Deviations from Plan

None - plan executed exactly as written.
