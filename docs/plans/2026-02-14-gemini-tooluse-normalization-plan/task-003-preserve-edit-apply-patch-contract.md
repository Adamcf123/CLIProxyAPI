# Task 003: preserve edit apply_patch contract

## Description

Keep `edit` normalization as the single canonical conversion path to `apply_patch`, with current safety checks and deterministic patch generation unchanged.

## Execution Context

**Task Number**: 003 of 007  
**Phase**: Core Features  
**Prerequisites**: Existing edit normalizer behavior and tests are present.

## BDD Scenario Reference

**Spec**: `../2026-02-14-gemini-tooluse-normalization-design/bdd-specs.md`  
**Scenario**: Scenario 3: Keep edit conversion behavior unchanged

## Files to Modify/Create

- Modify: `internal/runtime/executor/claude_executor.go`
- Modify: `internal/runtime/executor/tool_call_normalizer.go` (only if required for compatibility)
- Modify: `internal/runtime/executor/claude_executor_test.go`
- Modify: `internal/runtime/executor/tool_call_normalizer_test.go`

## Semantic Stages

### Plan Quality Gate
- Confirm Scenario 3 assertions target behavior parity for edit conversion.
- Confirm RED failure is contract regression, not fixture/setup issues.
- Confirm assertions cover tool name, patchText presence, and deterministic content ordering.

### RED
- Add failing regression tests proving edit behavior drift after introducing generic non-edit split logic.

### GREEN
- Adjust routing between generic splitter and edit normalizer so edit stays authoritative.
- Re-run edit-focused and stream-focused tests to confirm pass.

### REFACTOR
- Consolidate branch conditions to avoid duplicate edit checks.
- Re-run targeted tests and full executor package tests.

## Verification Commands

```bash
go test ./internal/runtime/executor -run "TestToolCallNormalizer|TestClaudeExecutorFromEqualsToNormalization" -count=1
go test ./internal/runtime/executor -count=1
```

## Success Criteria

- Scenario 3 passes fully.
- `edit -> apply_patch` behavior remains backward-compatible.
- No path safety or operation validation regressions.
