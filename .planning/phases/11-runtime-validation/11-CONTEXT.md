# Phase 11: Runtime Validation (Optional) - Context

**Gathered:** 2026-02-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Validate runtime behavior under real-ish conditions (real traffic or load test):

- Metrics collection is non-blocking under load (no obvious throughput/latency regression).
- Stderr live output + summary and SQLite persistence work in the chosen runtime environment.
- Hard-to-unit-test streaming edge cases (e.g., terminal error after headers already committed) are exercised and conclusions are recorded.

</domain>

<decisions>
## Implementation Decisions

### Validation environment and safety guardrails
- Primary environment: local/dev environment, but run as an isolated instance (do not share a production deployment).
- Upstream provider calls: allowed to hit real providers (real network / real billing).
- Guardrail strategy: prioritize concurrency/QPS limiting as the main blast-radius control.
- Data isolation: use an independent deployment/instance for the validation run.

### Load profile
- Load level: light (roughly concurrency 1-5).
- Run shape: steady-state run (not ramp/step, not burst).
- Request mix: 50/50 streaming vs non-streaming.
- Provider/model coverage: cover 2-3 mainstream providers/models (not single-provider only, not full matrix).

### Pass/fail criteria and evidence
- Pass/fail style: use comparative thresholds (not purely subjective), but exact numeric thresholds are left to planner proposal.
- Evidence focus: resource utilization is the most important evidence class (CPU/memory/etc.), in addition to functional correctness.
- Deliverable format: Markdown report.
- Location: store artifacts under `.planning/phases/11-runtime-validation/` for auditability/repro.

### Must-cover edge scenarios
- Required scenarios (all must be covered and concluded in the report):
  - terminal error after headers committed
  - client cancel/disconnect (HTTP 499)
  - upstream no-usage path (ensurePublished still yields a DB row)
  - persistence degraded path (e.g., queue_full/insert_failure) with observability
- Coverage method: scriptable/repeatable runs (not one-off manual only).
- Primary observation point: SQLite row exists and is queryable, with correct classification/fields.
- Repetitions: run each required edge scenario at least 3 times.

### Claude's Discretion
- Exact numeric performance/resource thresholds, provided they are expressed as clear comparative thresholds and recorded in the report.
- The concrete script shape/commands, as long as they are reproducible and produce the agreed evidence.

</decisions>

<specifics>
## Specific Ideas

- Prefer a repeatable script-driven validation flow.
- Focus the report on reproducibility: commands, steps, and concise results.

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within phase scope.

</deferred>

---

*Phase: 11-runtime-validation*
*Context gathered: 2026-02-01*
