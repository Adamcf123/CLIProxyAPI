# Task 001: Red normalizer parser tests

- **type**: test-red
- **scenario-ref**: Scenario 1, Scenario 2, Scenario 4
- **target-file**: `internal/runtime/executor/tool_call_normalizer_test.go`
- **depends-on**: none

## What to do

- Add failing tests that define expected normalization behavior for concatenated `edit` JSON objects (`{...}{...}{...}`).
- Add failing tests that verify valid single-object `edit` payloads remain unchanged.
- Add failing tests that verify conversion supports more than three files and multiple replacements while preserving deterministic operation order.
- Use in-memory fixtures for tool payload fragments and SSE-like chunks; do not call network services.

## Verification

- Run: `go test ./internal/runtime/executor -run TestToolCallNormalizer -count=1`
- Expected in this Red step: new assertions fail before implementation.
