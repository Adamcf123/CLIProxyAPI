# Task 007: enforce model-route guard isolation

## Description

Verify the new all-tool normalization behavior remains strictly isolated to the existing guard (`from == to` and `gemini-3-flash-preview`).

## Execution Context

**Task Number**: 007 of 007  
**Phase**: Refinement  
**Prerequisites**: Core behavior for Scenarios 1-6 is implemented and tested.

## BDD Scenario Reference

**Spec**: `../2026-02-14-gemini-tooluse-normalization-design/bdd-specs.md`  
**Scenario**: Scenario 7: Keep behavior isolated by model and route

## Scenario-Test Mapping

- Scenario 7 Given/When/Then -> `TestClaudeExecutorFromEqualsToNormalization_GuardIsolation`
- Positive control in same fixture -> `TestClaudeExecutorFromEqualsToNormalization_GuardIsolation_GuardOnNormalizes`
- Required isolation matrix:
  - model mismatch only (`from == to`, non-gemini model)
  - route mismatch only (`from != to`, gemini model)
  - both mismatch (`from != to`, non-gemini model)
- Required assertions:
  - no split/error side effects outside guard
  - event sequence and payload equivalence preserved outside guard
  - test fixture stream is identical while only guard conditions vary
  - guard-on case in the same fixture must emit normalization signal as anchor

## Files to Modify/Create

- Modify: `internal/runtime/executor/claude_executor.go`
- Modify: `internal/runtime/executor/claude_executor_test.go`

## Semantic Stages

### Plan Quality Gate
- Confirm Scenario 7 checks both non-gemini model and `from != to` route.
- Confirm RED failures represent guard leakage, not unrelated test setup.
- Confirm assertions validate absence of split/error side effects outside guard.
- Confirm matrix coverage for single-factor and double-factor guard mismatch cases.
- Confirm same-fixture positive control proves normalization works when guard matches.

### RED
- Add failing guard-isolation tests for non-target model and route variants.
- RED failure signature must be semantic (guard leakage causing split/error mutation outside guarded path).

### GREEN
- Refine guard checks or branch placement to guarantee isolation.
- Re-run isolation tests and related normalization tests.

### REFACTOR
- Consolidate guard expression if needed for readability and maintainability.
- Re-run package tests to confirm no regressions.

## Verification Commands

```bash
go test ./internal/runtime/executor -run "TestClaudeExecutorFromEqualsToNormalization" -count=1
go test ./internal/runtime/executor -count=1
```

## Success Criteria

- Scenario 7 passes.
- No normalization behavior leaks into non-target model/route paths.
- Final executor test suite remains green.
