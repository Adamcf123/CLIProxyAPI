# Task 004: emit invalid_tool_input for malformed sequences

**depends-on**: task-002-split-non-edit-concatenated-object-sequences

## Description

Ensure malformed concatenated object sequences produce explicit parseable `invalid_tool_input` errors and do not forward corrupted payloads.

## Execution Context

**Task Number**: 004 of 007  
**Phase**: Core Features  
**Prerequisites**: Generic split path exists for non-edit concatenated input.

## BDD Scenario Reference

**Spec**: `../2026-02-14-gemini-tooluse-normalization-design/bdd-specs.md`  
**Scenario**: Scenario 4: Reject malformed concatenated payloads with explicit error

## Files to Modify/Create

- Modify: `internal/runtime/executor/claude_executor.go`
- Modify: `internal/runtime/executor/claude_executor_test.go`

## Semantic Stages

### Plan Quality Gate
- Confirm Scenario 4 maps to malformed-sequence error handling only.
- Confirm assertions check both error type and absence of corrupted forwarded args.
- Confirm error output does not leak raw sensitive payload content.

### RED
- Add failing stream tests for malformed concatenated non-edit payloads.
- Verify failures are behavior-semantic (missing invalid_tool_input or bad forwarding).

### GREEN
- Implement malformed sequence detection and explicit error emission using existing error contract.
- Ensure forwarding is blocked for corrupted payload lines.

### REFACTOR
- Deduplicate error mapping helpers if multiple branches use the same shape.
- Re-run all impacted tests.

## Verification Commands

```bash
go test ./internal/runtime/executor -run "TestNormalizeToolUseLine|TestClaudeExecutorFromEqualsToNormalization" -count=1
go test ./internal/runtime/executor -count=1
```

## Success Criteria

- Scenario 4 passes.
- Error event remains parseable and stable.
- Corrupted tool arguments are never emitted downstream.
