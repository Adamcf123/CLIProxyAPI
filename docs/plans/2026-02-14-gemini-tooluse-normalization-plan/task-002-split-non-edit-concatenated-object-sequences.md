# Task 002: split non-edit concatenated object sequences

**depends-on**: task-001-preserve-single-object-passthrough

## Description

Implement deterministic splitting for non-`edit` `tool_use` payloads when the reconstructed input is a top-level object sequence (for example `{...}{...}{...}`).

## Execution Context

**Task Number**: 002 of 007  
**Phase**: Core Features  
**Prerequisites**: Scenario 1 passthrough path is stable and covered.

## BDD Scenario Reference

**Spec**: `../2026-02-14-gemini-tooluse-normalization-design/bdd-specs.md`  
**Scenario**: Scenario 2: Split concatenated non-edit objects into independent calls

## Scenario-Test Mapping

- Scenario 2 Given/When/Then -> `TestNormalizeToolUseLine_NonEditConcatenatedObjectsSplit`
- Stream sequence verification -> `TestClaudeExecutorFromEqualsToNormalization/non-edit concatenated payload split`
- Required assertions:
  - exact split object count equals top-level object count
  - each emitted tool block contains exactly one JSON object input
  - emitted object order equals original order
  - deterministic index/id allocation without collision

## Files to Modify/Create

- Modify: `internal/runtime/executor/claude_executor.go`
- Modify: `internal/runtime/executor/claude_executor_test.go`
- Optional create: `internal/runtime/executor/tool_use_stream_splitter.go`

## Semantic Stages

### Plan Quality Gate
- Confirm split behavior maps only to Scenario 2 and does not alter edit strategy.
- Confirm assertions verify object count, order, and one-object-per-call invariants.
- Confirm no weak assertion patterns (`A || B`) in expected stream output.
- Confirm index/id assertions bind to executor stream state source.

### RED
- Add failing tests for non-edit concatenated payloads requiring multi-call split output.
- Capture expected failure location in guarded stream normalization.
- RED failure signature must be semantic (missing split, wrong order, one-call merge, or index/id drift).

### GREEN
- Implement split logic using top-level object sequence parsing with deterministic ordering.
- Ensure additional emitted calls are valid tool block sequences.
- Re-run target tests and confirm pass.

### REFACTOR
- Extract helper functions for index/id allocation and split event assembly.
- Keep behavior unchanged while improving readability.

## Verification Commands

```bash
go test ./internal/runtime/executor -run "TestClaudeExecutorFromEqualsToNormalization|TestNormalizeToolUseLine" -count=1
go test ./internal/runtime/executor -count=1
```

## Success Criteria

- Scenario 2 passes with deterministic split output.
- Non-edit concatenated payloads no longer produce ambiguous merged arguments.
- Existing tests remain green.
