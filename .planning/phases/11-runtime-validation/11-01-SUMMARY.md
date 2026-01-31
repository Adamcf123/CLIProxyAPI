---
phase: 11-runtime-validation
plan: 01
subsystem: testing
tags: [bash, go, curl, jq, sqlite3, metrics_summary]

# Dependency graph
requires:
  - phase: 10-request-id-robustness
    provides: tracking_id/request_id correlation + SQLite persistence invariants
provides:
  - Scripted baseline + edge-case runtime validation harness with artifact isolation
  - Local mock upstream + cancel client tools for repeatable edge-case reproduction
  - Audit-ready Markdown report template
affects: [11-runtime-validation, validation, release-audit]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Phase validation scripts isolate server CWD under per-run artifacts/"
    - "request_id is derived from metrics_summary.tracking_id for evidence correlation"

key-files:
  created:
    - .planning/phases/11-runtime-validation/scripts/lib.sh
    - .planning/phases/11-runtime-validation/scripts/run_baseline.sh
    - .planning/phases/11-runtime-validation/scripts/run_edge_cases.sh
    - .planning/phases/11-runtime-validation/tools/mock_openai_compat_upstream.go
    - .planning/phases/11-runtime-validation/tools/cancel_stream_client.go
    - .planning/phases/11-runtime-validation/11-REPORT-TEMPLATE.md
    - .planning/phases/11-runtime-validation/.gitignore
  modified: []

key-decisions:
  - "Use phase-local artifacts/ run directories to prevent root logs/ pollution"
  - "Use metrics_summary tracking_id as request_id to correlate stderr ↔ SQLite ↔ management"

patterns-established:
  - "All runtime validation artifacts must be gitignored (DB/logs/tmp/config/pids)"
  - "Scripts must fail-loud on missing deps and on secret-like material detected in artifacts"

# Metrics
duration: 8 min
completed: 2026-02-01
---

# Phase 11 Plan 01: Runtime Validation Harness Summary

**Baseline + edge-case runtime validation can now be reproduced via single commands, with per-run artifact isolation and audit-ready evidence capture.**

## Performance

- **Duration:** 8 min
- **Started:** 2026-01-31T18:49:12Z
- **Completed:** 2026-01-31T18:57:15Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments

- Baseline runner (`.planning/phases/11-runtime-validation/scripts/run_baseline.sh`) builds + starts an isolated server, runs a light steady-state mix, and captures evidence (resources, curl timings, SQLite snapshot, metrics_summary sample).
- Edge-case runner (`.planning/phases/11-runtime-validation/scripts/run_edge_cases.sh`) scripts the four required scenarios with >=3 repeats each and writes an evidence table keyed by request_id.
- Report template (`.planning/phases/11-runtime-validation/11-REPORT-TEMPLATE.md`) standardizes audit-ready writeups and explicitly bans secret disclosure.

## Task Commits

Each task was committed atomically:

1. **Task 1: 创建 Phase 11 验证脚本与产物隔离约束** - `9e5706b` (feat)
2. **Task 2: 增加 edge-case 复现工具（覆盖全部必测边界）** - `208a4a0` (feat)

## Files Created/Modified

- `.planning/phases/11-runtime-validation/.gitignore` - Ensures artifacts/ and common runtime outputs are never tracked.
- `.planning/phases/11-runtime-validation/scripts/lib.sh` - Shared helpers: deps check, per-run dir, build/start/stop, resource sampling, secret scan.
- `.planning/phases/11-runtime-validation/scripts/run_baseline.sh` - Baseline run harness with concurrency+QPS guardrails and evidence capture.
- `.planning/phases/11-runtime-validation/scripts/run_edge_cases.sh` - Edge scenarios runner with request_id correlation + evidence table.
- `.planning/phases/11-runtime-validation/tools/mock_openai_compat_upstream.go` - Mock upstream with SSE then hard disconnect; non-streaming path omits usage.
- `.planning/phases/11-runtime-validation/tools/cancel_stream_client.go` - Client that cancels a streaming request after reading N bytes.
- `.planning/phases/11-runtime-validation/11-REPORT-TEMPLATE.md` - Audit-ready Markdown template.

## Decisions Made

- Use per-run `artifacts/run-*` as the sole runtime output location (server CWD = run_dir) to keep `logs/metrics.db` isolated.
- Standardize on `request_id = metrics_summary.tracking_id` for all evidence tables and checks.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

Set environment variables for local runtime validation:

- `API_KEY` (client key; placeholder ok; must not be committed)
- `MANAGEMENT_KEY` (validation-only management key; must not be committed)

## Next Phase Readiness

- Ready for `.planning/phases/11-runtime-validation/11-02-PLAN.md` (execute runtime validation and produce the final audit report).

---
*Phase: 11-runtime-validation*
*Completed: 2026-02-01*
