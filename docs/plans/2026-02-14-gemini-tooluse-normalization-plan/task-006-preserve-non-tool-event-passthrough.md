# Task 006: preserve non-tool event passthrough

## Description

Ensure text/thinking/other non-target SSE events continue to pass through without semantic changes while tool normalization is enabled.

## Execution Context

**Task Number**: 006 of 007  
**Phase**: Integration  
**Prerequisites**: Tool split and error handling logic are in place.

## BDD Scenario Reference

**Spec**: `../2026-02-14-gemini-tooluse-normalization-design/bdd-specs.md`  
**Scenario**: Scenario 6: Preserve non-tool content events

## Files to Modify/Create

- Modify: `internal/runtime/executor/claude_executor.go`
- Modify: `internal/runtime/executor/claude_executor_test.go`

## Semantic Stages

### Plan Quality Gate
- Confirm Scenario 6 is validated by event-type-specific passthrough assertions.
- Confirm RED failure is semantic mutation of non-tool events, not unrelated setup error.
- Confirm assertions cover ordering and payload equivalence for non-tool events.

### RED
- Add failing tests with mixed event streams (thinking/text/tool events) under guarded path.

### GREEN
- Adjust routing so only target tool events enter split/normalize branches.
- Keep non-tool event forwarding unchanged.

### REFACTOR
- Reduce branching complexity while preserving event type isolation.
- Re-run mixed-stream tests and full executor tests.

## Verification Commands

```bash
go test ./internal/runtime/executor -run "TestNormalizeToolUseLine|TestClaudeExecutorFromEqualsToNormalization" -count=1
go test ./internal/runtime/executor -count=1
```

## Success Criteria

- Scenario 6 passes.
- Non-tool event semantics and ordering remain unchanged.
- No unintended side effects introduced by tool normalization.
