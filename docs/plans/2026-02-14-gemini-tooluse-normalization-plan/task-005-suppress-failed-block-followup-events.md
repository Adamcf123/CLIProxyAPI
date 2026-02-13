# Task 005: suppress failed block follow-up events

**depends-on**: task-004-emit-invalid-tool-input-for-malformed-sequences

## Description

Guarantee failed block suppression remains block-local and correctly drops only follow-up events for the failed index until block stop.

## Execution Context

**Task Number**: 005 of 007  
**Phase**: Integration  
**Prerequisites**: Malformed sequence handling emits `invalid_tool_input` consistently.

## BDD Scenario Reference

**Spec**: `../2026-02-14-gemini-tooluse-normalization-design/bdd-specs.md`  
**Scenario**: Scenario 5: Suppress follow-up events only for the failed block

## Scenario-Test Mapping

- Scenario 5 Given/When/Then -> `TestNormalizeToolUseLine_SuppressesOnlyFailedBlockIndex`
- Interleaved stream verification -> `TestClaudeExecutorFromEqualsToNormalization/failed block suppression isolation`
- Required assertions:
  - failed index follow-up `content_block_delta` and `content_block_stop` are dropped
  - non-failed index events continue unchanged
  - suppression state clears only when failed index receives `content_block_stop`
  - failure index source equals the index emitted on `invalid_tool_input`

## Files to Modify/Create

- Modify: `internal/runtime/executor/claude_executor.go`
- Modify: `internal/runtime/executor/claude_executor_test.go`

## Semantic Stages

### Plan Quality Gate
- Confirm Scenario 5 checks suppression boundaries by index.
- Confirm assertions include both dropped same-index events and preserved other-index events.
- Confirm no weak pass conditions in suppression assertions.
- Confirm assertions enforce failed-index source alignment and precise clear timing.

### RED
- Add failing tests with interleaved block indices where one index fails and others continue.
- RED failure signature must be semantic (cross-index suppression leakage or wrong clear timing).

### GREEN
- Implement or adjust suppression state handling to clear precisely on failed block stop.
- Verify unaffected indices continue with unchanged behavior.

### REFACTOR
- Simplify suppression map lifecycle and cleanup logic.
- Re-run targeted and package tests.

## Verification Commands

```bash
go test ./internal/runtime/executor -run "TestNormalizeToolUseLine_SuppressesOnlyFailedBlockIndex|TestNormalizeToolUseLine" -count=1
go test ./internal/runtime/executor -count=1
```

## Success Criteria

- Scenario 5 passes with block-local suppression.
- No cross-index suppression leakage.
- Existing suppression behavior for edit regressions remains intact.
