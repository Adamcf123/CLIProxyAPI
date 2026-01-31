# Phase 11 Runtime Validation Report (Template)

IMPORTANT SECURITY NOTE:

- DO NOT include API keys, tokens, or raw auth headers in this report.
- DO NOT paste `Authorization:` or `X-Management-Key:` header lines.
- Link to evidence files under `.planning/phases/11-runtime-validation/artifacts/` instead.

---

## Environment

- **Date:** YYYY-MM-DD
- **Machine/OS:**
- **Repo:** CLIProxyAPI
- **Git commit:** `<git rev-parse HEAD>`
- **Go version:** `go version`

## Commands (Repro Steps)

Baseline:

```bash
export API_KEY="sk-dummy"   # placeholder only
bash .planning/phases/11-runtime-validation/scripts/run_baseline.sh --models gpt-5.2,minimax-m2.1,glm-4.7
```

Edge cases:

```bash
export MANAGEMENT_KEY="phase11-dev"  # validation-only key
export API_KEY="sk-dummy"            # placeholder only
bash .planning/phases/11-runtime-validation/scripts/run_edge_cases.sh all
```

## Load Profile

- **Shape:** steady-state
- **Concurrency:**
- **QPS limit:**
- **Duration:**
- **Request mix:** 50/50 streaming vs non-streaming
- **Provider/model coverage:** (list actual provider/model set used)

## Evidence Index

Each run writes a dedicated directory under:

- `.planning/phases/11-runtime-validation/artifacts/run-YYYYMMDD-HHMMSS-<label>/`

For each run, link:

- `run_meta.json` (command + config + git commit)
- `server_resources.tsv` (CPU/mem/rss/vsz/etime)
- `curl_timings.tsv` (http_code/connect/total)
- `sqlite_metrics_snapshot.txt` (COUNT(*) + last 5 rows)
- `server.stderr.log` / `metrics_summary_sample.txt` (for request_id correlation)

## Results

### Baseline Result

- **Pass/Fail:** [PASS | FAIL]
- **Rationale:**
- **Notes:**

### Edge Scenarios

All edge scenarios must be executed >= 3 times.

#### 1) terminal error after headers committed

- **Pass/Fail:** [PASS | FAIL]
- **Expected:** client receives partial SSE, then connection terminates; server records a failure semantics row.
- **Evidence:**
- **Notes:**

#### 2) client cancel/disconnect (HTTP 499)

- **Pass/Fail:** [PASS | FAIL]
- **Expected:** SQLite row exists with `status_code=499` and empty/NULL `error_info`.
- **Evidence:**
- **Notes:**

#### 3) upstream no-usage path

- **Pass/Fail:** [PASS | FAIL]
- **Expected:** SQLite row exists even if tokens are NULL; `metrics_summary.usage_note` indicates usage missing.
- **Evidence:**
- **Notes:**

#### 4) persistence degraded (deterministic insert_failure)

- **Pass/Fail:** [PASS | FAIL]
- **Expected:** management metrics response includes `meta.persistence.degraded=true` and a drop reason like `insert_failure`.
- **Evidence:**
- **Notes:**

## Conclusion

- **Overall:** [PASS | FAIL]
- **Confidence:**
- **Follow-ups:**
