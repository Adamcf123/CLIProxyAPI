# Task 002: Green normalizer helper implementation

- **type**: implementation-green
- **scenario-ref**: Scenario 1, Scenario 2, Scenario 4
- **target-file**: `internal/runtime/executor/tool_call_normalizer.go`
- **depends-on**: `task-001-red-normalizer-parser-tests.md`

## What to do

- Implement a normalizer that detects concatenated top-level JSON objects in `edit` tool input.
- Implement deterministic conversion rules that merge parsed `edit` objects into one `apply_patch` input object with `patchText`.
- Preserve pass-through behavior for already valid single-object `edit` payloads.
- Ensure conversion supports any number of files and hunks, not a fixed cap.

## Verification

- Run: `go test ./internal/runtime/executor -run TestToolCallNormalizer -count=1`
- Expected in this Green step: tests from Task 001 pass.
