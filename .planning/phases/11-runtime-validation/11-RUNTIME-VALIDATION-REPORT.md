# Phase 11 Runtime Validation Report

IMPORTANT SECURITY NOTE:

- DO NOT include API keys, tokens, or raw auth headers in this report.
- DO NOT paste `Authorization:` or `X-Management-Key:` header lines.
- Link to evidence files under `.planning/phases/11-runtime-validation/artifacts/` instead.

---

## Environment

- **Date:** 2026-02-01
- **Machine/OS:** linux/amd64 (local)
- **Repo:** CLIProxyAPI
- **Git commit:** `03bac093361aef17861a60e9b9c782e0a9d3ad1b`
- **Go version:** `go version go1.24.0 linux/amd64`

## Commands (Repro Steps)

Baseline (real-ish providers, light steady-state):

```bash
# Client API key for local proxy auth only (placeholder; do not commit real keys)
export API_KEY="sk-dummy"

# NOTE: used an alternate port because 53355 was already in use.
bash .planning/phases/11-runtime-validation/scripts/run_baseline.sh \
  --port 53366 \
  --concurrency 2 \
  --duration-sec 15 \
  --qps 0.6 \
  --models gpt-5.2,minimax-m2.1,glm-4.7
```

Edge cases (mock upstream, reproducible, no real billing expected):

```bash
# Validation-only management key used ONLY for this isolated run_dir config.
# Do NOT commit real values.
export MANAGEMENT_KEY="<redacted>"
export API_KEY="sk-dummy"

bash .planning/phases/11-runtime-validation/scripts/run_edge_cases.sh all
```

## Load Profile

- **Shape:** steady-state
- **Concurrency:** 2
- **QPS limit:** 0.6 total (across workers)
- **Duration:** 15s
- **Request mix:** 50/50 streaming vs non-streaming
- **Provider/model coverage (requested):**
  - `gpt-5.2` (via provider `codex`)
  - `minimax-m2.1` (via provider `iflow`)
  - `glm-4.7` (via providers `claude` + `iflow`)

## Evidence Index

This report is audit-oriented: it links evidence files instead of pasting logs.

### Baseline Run Evidence

- **run_dir:** `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/`
- `run_meta.json`: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/run_meta.json`
- `server.stdout.log`: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/server.stdout.log`
- `server.stderr.log`: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/server.stderr.log`
- `metrics_summary_sample.txt` (request_id correlation): `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/metrics_summary_sample.txt`
- `server_resources.tsv`: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/server_resources.tsv`
- `server_resources_summary.txt`: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/server_resources_summary.txt`
- `curl_timings.tsv`: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/curl_timings.tsv`
- `curl_timings_summary.txt`: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/curl_timings_summary.txt`
- `logs/metrics.db`: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/logs/metrics.db`
- `sqlite_metrics_snapshot.txt`: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/sqlite_metrics_snapshot.txt`
- `sqlite_baseline_checks.txt` (counts + sample rows): `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/sqlite_baseline_checks.txt`
- `persistence_log_scan.txt` (stderr keyword scan): `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/persistence_log_scan.txt`

### Edge Case Run Evidence

TBD (filled in Task 2).

## Pass/Fail Thresholds

These are comparative / guardrail-style thresholds (planner discretion), recorded for audit repeatability.

Baseline should PASS if all are true:

1) **Resource stability** (from `server_resources_summary.txt`)
   - RSS peak delta <= 50 MB (peak RSS - first RSS)
   - CPU peak <= 200% (single process across cores) and median <= 50%

2) **Request-side latency** (from `curl_timings_summary.txt`)
   - `time_total` p95 <= 10s
   - `time_total` max <= 20s

3) **Persistence baseline sanity**
   - `logs/metrics.db` exists
   - `metrics` row count > 0 for the run
   - sample rows show non-NULL `status_code`

Edge scenarios should PASS if:

- Each required scenario has >= 3 repetitions in `edge_evidence.tsv`
- Each repetition has a queryable SQLite row (or for persistence-degraded: management meta evidence)
- Observed semantics match expectations (status/error_info/tokens nullability / degraded meta)

## Results

### Baseline Result

- **Pass/Fail:** PASS
- **Rationale:**
  - Resources stay stable under light steady-state: RSS peak delta is ~4 MB and CPU median is ~1.4%.
  - Request latency is within guardrails: p95 total is ~3.24s (max ~3.67s).
  - SQLite persistence is working in-run: `metrics.db` exists and has 6 rows with non-NULL `status_code`.
- **Evidence:**
  - Resources: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/server_resources_summary.txt`
  - Timings: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/curl_timings_summary.txt`
  - SQLite checks: `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/sqlite_baseline_checks.txt`

### Edge Scenarios

TBD (filled in Task 2).

## Conclusion

- **Overall:** PASS (baseline only)
- **Confidence:** Moderate for baseline stability under the tested runtime profile.
- **Follow-ups:**
  - Execute required edge scenarios (Task 2) and then finalize overall PASS/FAIL.
