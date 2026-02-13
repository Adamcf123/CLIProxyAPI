# Task 001: preserve single-object passthrough

## Description

Ensure non-`edit` `tool_use` payloads containing exactly one valid JSON object are forwarded unchanged on the guarded normalization path.

## Execution Context

**Task Number**: 001 of 007  
**Phase**: Core Features  
**Prerequisites**: Existing stream normalization entrypoint is reachable in `internal/runtime/executor/claude_executor.go`.

## BDD Scenario Reference

**Spec**: `../2026-02-14-gemini-tooluse-normalization-design/bdd-specs.md`  
**Scenario**: Scenario 1: Preserve valid single-object non-edit tool input

## Files to Modify/Create

- Modify: `internal/runtime/executor/claude_executor.go`
- Modify: `internal/runtime/executor/claude_executor_test.go`

## Semantic Stages

### Plan Quality Gate
- Confirm one-to-one mapping between Scenario 1 and the target non-edit passthrough tests.
- Confirm RED failure target is behavioral mismatch, not setup/import failure.
- Confirm assertions validate unchanged tool name and unchanged input payload.

### RED
- Add/extend stream tests where non-edit tool input is a single JSON object.
- Verify current behavior fails against the exact passthrough assertions for the guarded path.

### GREEN
- Implement minimal executor changes to keep single-object non-edit payload byte-equivalent through normalization.
- Re-run target tests to confirm pass.

### REFACTOR
- Simplify local branches without changing behavior.
- Re-run target and related executor stream tests.

## Verification Commands

```bash
go test ./internal/runtime/executor -run "TestNormalizeToolUseLine|TestClaudeExecutorFromEqualsToNormalization" -count=1
go test ./internal/runtime/executor -count=1
```

## Success Criteria

- Scenario 1 assertions pass.
- No semantic change for single-object non-edit tool calls.
- Existing executor tests remain green.
