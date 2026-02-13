# Task 003: Red from-equals-to stream tests

- **type**: test-red
- **scenario-ref**: Scenario 3, Scenario 5
- **target-file**: `internal/runtime/executor/claude_executor_test.go`
- **depends-on**: none

## What to do

- Add failing tests for the `from == to` streaming path that cover model-gated normalization behavior.
- Add failing tests that require safe failure when concatenated payloads are ambiguous and cannot be converted losslessly.
- Add failing tests that verify non-matching model or route does not trigger conversion.
- Use test doubles for upstream stream source and chunk forwarding; avoid real HTTP/network interactions.

## Verification

- Run: `go test ./internal/runtime/executor -run TestClaudeExecutorFromEqualsToNormalization -count=1`
- Expected in this Red step: new assertions fail before wiring changes.
