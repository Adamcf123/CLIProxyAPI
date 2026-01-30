# Phase 6 Context: Guaranteed Usage Publish (Persistence Reliability)

## Why This Phase Exists

Milestone v1 audit found a gap in end-to-end persistence reliability:

- SQLite persistence is triggered via MetricsPlugin, which runs only when a usage record is published.
- Some executor paths can legitimately end without any usage tokens (especially streaming), and the publish logic skips records when all tokens are zero and the request is not marked failed.
- Result: a request can complete (or fail) and still produce no DB row, breaking historical queries and auditability.

This phase closes the gap by guaranteeing exactly one usage record is published per request even when upstream does not return any usage fields.

## Scope

- Ensure streaming executors call `reporter.ensurePublished(ctx)` at the end of the stream to guarantee a usage record.
- Ensure failure paths still publish a failure record (`publishFailure`) and are not preempted by ensurePublished.
- Add targeted tests (or integration-style tests) that prove: "no usage metadata" still results in a persisted DB row.

## Out of Scope

- Changing the best-effort nature of persistence under overload (queue-full drop remains by design).
- Expanding metrics beyond v1 scope.
