# gemini tool_use normalization implementation plan

> **For OpenCode:** REQUIRED SUB-SKILL: Use Skill tool to load `superpowers:executing-plans` and execute this plan task-by-task.

**Goal:** Implement a canonical stream normalization path that supports all `tool_use` calls for concatenated object payloads on guarded gemini from==to streaming, while preserving existing `edit -> apply_patch` behavior.

**Architecture:** Keep normalization in `internal/runtime/executor/claude_executor.go` from==to stream flow, preserve `ToolCallNormalizer` as the only edit conversion authority, and add deterministic non-edit split behavior with explicit fail-loud error events.

**Tech Stack:** Go, existing executor SSE event handling, existing JSON scanners in executor/normalizer tests.

**Design Support:**
- [BDD Specs](../2026-02-14-gemini-tooluse-normalization-design/bdd-specs.md)
- [Architecture](../2026-02-14-gemini-tooluse-normalization-design/architecture.md)
- [Best Practices](../2026-02-14-gemini-tooluse-normalization-design/best-practices.md)

## Execution Plan

- [Task 001: preserve single-object passthrough](./task-001-preserve-single-object-passthrough.md)
- [Task 002: split non-edit concatenated object sequences](./task-002-split-non-edit-concatenated-object-sequences.md)
- [Task 003: preserve edit apply_patch contract](./task-003-preserve-edit-apply-patch-contract.md)
- [Task 004: emit invalid_tool_input for malformed sequences](./task-004-emit-invalid-tool-input-for-malformed-sequences.md)
- [Task 005: suppress failed block follow-up events](./task-005-suppress-failed-block-followup-events.md)
- [Task 006: preserve non-tool event passthrough](./task-006-preserve-non-tool-event-passthrough.md)
- [Task 007: enforce model-route guard isolation](./task-007-enforce-model-route-guard-isolation.md)

---

## Execution Handoff

Plan complete. Execution options:

1. Orchestrated Execution - Use Skill tool to load `superpowers:executing-plans`.
2. BDD-Focused Execution - Use Skill tool to load `superpowers:behavior-driven-development`.
