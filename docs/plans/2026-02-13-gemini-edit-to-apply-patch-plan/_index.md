# Plan: gemini edit-to-apply-patch normalization

## Goal

Ensure `gemini-3-flash-preview` outputs on `from == to` can safely handle malformed concatenated `edit` JSON objects by producing one valid `apply_patch` payload that supports any number of files and hunks.

## Constraints

- BDD scenarios in `../2026-02-13-gemini-edit-to-apply-patch-design/bdd-specs.md` are the source of truth.
- Test-first flow is required: each Red task must be completed before its paired Green task.
- Unit and integration tests must isolate external dependencies via test doubles; no real network calls.
- Conversion applies only to matching model and route; all other traffic remains unchanged.

## Commit Boundaries

- Commit A: Red tests for normalizer helper (`task-001`).
- Commit B: Green helper implementation (`task-002`).
- Commit C: Red executor stream-path tests (`task-003`).
- Commit D: Green executor wiring and safety behavior (`task-004`).

## Execution Plan

- [Task 001: Red normalizer parser tests](./task-001-red-normalizer-parser-tests.md)
- [Task 002: Green normalizer helper implementation](./task-002-green-normalizer-helper-implementation.md)
- [Task 003: Red from-equals-to stream tests](./task-003-red-from-equals-to-stream-tests.md)
- [Task 004: Green executor wiring and safety behavior](./task-004-green-executor-wiring-and-safety.md)
