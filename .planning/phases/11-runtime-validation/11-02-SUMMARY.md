---
phase: 11-runtime-validation
plan: 02
subsystem: testing
tags: [runtime-validation, bash, sqlite, metrics, go, curl]

# Dependency graph
requires:
  - phase: 11-runtime-validation
    provides: scripted baseline + edge-case harness (11-01)
provides:
  - Audit-ready runtime validation report with baseline + edge scenarios evidence
  - Deterministic edge-case harness and correlation between stderr metrics_summary and SQLite rows
affects: [release-readiness, runtime-validation]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "metrics_summary.tracking_id aligns with SQLite metrics.request_id for audit correlation"
    - "OpenAI-compat streaming EOF without [DONE] is treated as a terminal error"
    - "Usage publish falls back to Gin writer status_code when handler tail runs later"

key-files:
  created:
    - .planning/phases/11-runtime-validation/11-RUNTIME-VALIDATION-REPORT.md
  modified:
    - .planning/phases/11-runtime-validation/scripts/run_edge_cases.sh
    - .planning/phases/11-runtime-validation/tools/mock_openai_compat_upstream.go
    - internal/metricsruntime/request_state.go
    - internal/metricsruntime/usage_plugin.go
    - internal/runtime/executor/openai_compat_executor.go

key-decisions:
  - "Baseline uses light steady-state (concurrency=2, qps=0.6, 15s) and absolute guardrail thresholds for PASS/FAIL."
  - "Persistence degraded scenario triggers queue_full/insert_failure via burst load under DB lock, to produce deterministic meta.persistence evidence."

patterns-established:
  - "Phase 11 evidence-first reporting: report links artifact paths instead of pasting logs"

# Metrics
duration: 25 min
completed: 2026-02-01
---

# Phase 11 Plan 02: Runtime Validation Execution Summary

**Baseline (real-ish providers) + edge-case scenarios executed with audit-ready evidence links, plus fixes to make stderr/SQLite correlation and edge-case observability deterministic.**

## Performance

- **Duration:** 25 min
- **Started:** 2026-01-31T19:00:55Z
- **Completed:** 2026-01-31T19:26:13Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- Baseline steady-state run captured resources/latency/SQLite evidence and recorded PASS thresholds in the report (`.planning/phases/11-runtime-validation/11-RUNTIME-VALIDATION-REPORT.md`).
- Required edge scenarios executed >=3 times each with SQLite-first evidence and management meta persistence degraded observability (artifacts linked from the report).
- Fixed correlation + semantics bugs discovered during runtime validation so the evidence is actually reproducible and comparable across stderr/SQLite.

## Task Commits

Each task was committed atomically:

1. **Task 1: 运行 baseline（真实 providers，轻负载 steady-state）并采集证据** - `56f8078` (fix)
2. **Task 2: 运行 required edge scenarios（每个 >=3 次）并记录结论** - `a0cc868` (fix)

## Checkpoint Outcome

- Human verification (report content + redaction): approved.

## Files Created/Modified

- `.planning/phases/11-runtime-validation/11-RUNTIME-VALIDATION-REPORT.md` - Audit-ready report with commands, thresholds, conclusions, and evidence paths
- `.planning/phases/11-runtime-validation/scripts/run_edge_cases.sh` - Deterministic edge-case runner with repeatable evidence generation
- `.planning/phases/11-runtime-validation/tools/mock_openai_compat_upstream.go` - Mock upstream that supports scenario-specific streaming behaviors
- `internal/metricsruntime/request_state.go` - Correlate metrics_summary tracking_id with request_id
- `internal/metricsruntime/usage_plugin.go` - Persist status_code reliably even when publish happens before handler tail
- `internal/runtime/executor/openai_compat_executor.go` - Treat missing [DONE] as terminal error and set error_info deterministically

## Decisions Made

- Used absolute guardrail thresholds (RSS delta, CPU, latency p95/max) for baseline PASS/FAIL to avoid subjective interpretation.
- For persistence degraded evidence, preferred deterministic drop triggers (queue_full / insert_failure) with management meta snapshots over inferring from logs.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Baseline default port 53355 already in use**

- **Found during:** Task 1
- **Issue:** `run_baseline.sh` failed with port-in-use
- **Fix:** Ran baseline with `--port 53366` and documented in report
- **Committed in:** `56f8078`

**2. [Rule 3 - Blocking] Edge-case temp config missed auth-dir, server failed to start**

- **Found during:** Task 2
- **Issue:** Edge-case run crashed with "failed to create auth directory"
- **Fix:** Added `auth-dir: "./auth"` in edge-case temp config
- **Committed in:** `a0cc868`

**3. [Rule 1 - Bug] stderr metrics_summary tracking_id was not correlatable to SQLite request_id**

- **Found during:** Task 2 (blocked scripted correlation)
- **Issue:** metrics_summary used a UUID while SQLite used 16-char hex request_id
- **Fix:** AttachRequestState now prefers Gin request_id as TrackingID
- **Committed in:** `56f8078`

**4. [Rule 1 - Bug] SQLite status_code was NULL for all rows in baseline**

- **Found during:** Task 1 evidence review
- **Issue:** usage publish happened before handler tail set RequestState.StatusCode
- **Fix:** usage plugin falls back to Gin writer status when RequestState.StatusCode is still 0
- **Committed in:** `56f8078`

**5. [Rule 1 - Bug] OpenAI-compat streaming EOF without [DONE] was treated as success**

- **Found during:** Task 2 terminal-error-after-headers scenario
- **Issue:** Executor did not enforce [DONE] and ForwardStream wrote its own [DONE], masking failure
- **Fix:** OpenAI-compat executor treats clean EOF without [DONE] as ErrUnexpectedEOF and sets RequestState error_info before ensurePublished
- **Committed in:** `a0cc868`

**6. [Rule 1 - Bug] Persistence degraded scenario was non-deterministic**

- **Found during:** Task 2 persistence-degraded scenario
- **Issue:** DB lock alone did not reliably cause drops
- **Fix:** Burst load under DB lock to deterministically trigger queue_full/insert_failure and capture management meta snapshots (>=3)
- **Committed in:** `a0cc868`

---

**Total deviations:** 6 auto-fixed (4 bug fixes, 2 blocking fixes)
**Impact on plan:** All auto-fixes were necessary for the runtime validation report to be reproducible/auditable and to cover required edge semantics.

## Issues Encountered

- Some failed edge-case attempts left stale local processes on ports 53356/53357; cleaned up and re-ran successfully.

## User Setup Required

None - this plan used local runtime and validation-only placeholders; no new external service setup was introduced.

## Next Phase Readiness

- None - checkpoint passed; Phase 11 is complete.

---
*Phase: 11-runtime-validation*
*Completed: 2026-02-01*
