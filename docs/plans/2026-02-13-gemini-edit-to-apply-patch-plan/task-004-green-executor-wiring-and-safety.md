# Task 004: Green executor wiring and safety behavior

- **type**: implementation-green
- **scenario-ref**: Scenario 1, Scenario 2, Scenario 3, Scenario 5
- **target-file**: `internal/runtime/executor/claude_executor.go`
- **depends-on**: `task-002-green-normalizer-helper-implementation.md`, `task-003-red-from-equals-to-stream-tests.md`

## What to do

- Wire normalization into the `from == to` streaming branch only when model is `gemini-3-flash-preview`.
- Apply conversion output from the helper before forwarding stream chunks.
- Enforce safe failure semantics for ambiguous payloads so corrupted tool arguments are never emitted.
- Keep all non-matching traffic behavior unchanged.

## Verification

- Run: `go test ./internal/runtime/executor -run TestClaudeExecutorFromEqualsToNormalization -count=1`
- Run: `go test ./internal/runtime/executor -run TestToolCallNormalizer -count=1`
- Run: `go test ./internal/runtime/executor -count=1`
- Expected in this Green step: all targeted tests pass.
