# Design: gemini tool_use normalization for all opencode tools

## Context

The target is to support all opencode `tool_use` calls on the `gemini-3-flash-preview` path where `from == to`, not only `edit`.

The current implementation only normalizes `edit` by converting concatenated JSON objects into one `apply_patch` payload. Other tools can still receive concatenated object sequences like `{...}{...}`, which produces ambiguous tool arguments.

User-confirmed decisions:

- Scope: all `tool_use` calls.
- Approach: keep `edit -> apply_patch`; for non-`edit`, automatically split concatenated top-level objects into multiple independent tool calls.

## Discovery Results

- Normalization is currently enabled only when `from == to` and model is `gemini-3-flash-preview` in `internal/runtime/executor/claude_executor.go`.
- `normalizeToolUsePayload` currently returns early unless `tool_name == "edit"` in `internal/runtime/executor/claude_executor.go`.
- `ToolCallNormalizer.NormalizeToolCall` is intentionally `edit`-specific in `internal/runtime/executor/tool_call_normalizer.go`.
- Existing stream safety semantics already exist for explicit tool normalization failures (`invalid_tool_input`) and same-index suppression in `internal/runtime/executor/claude_executor.go`.

## Requirements

1. Support all `tool_use` payloads under the same activation guard (`from == to` and `gemini-3-flash-preview`).
2. Preserve current `edit -> apply_patch` behavior and safety checks.
3. For non-`edit`, detect concatenated top-level object sequences in tool input and split them into multiple independent calls.
4. Keep SSE event ordering valid and deterministic.
5. Keep error behavior fail-loud and parseable (`invalid_tool_input`), while isolating failures to the affected content block.
6. Avoid broad behavioral changes outside the guarded path.
7. Respect the operational constraint: diagnostics and validation must not rely on reading whole log files.

## Rationale

This design provides one canonical strategy for malformed concatenated tool payloads while minimizing regression risk:

- Reuse proven `edit` normalization as-is.
- Add one stream-level splitter for non-`edit` tools instead of per-tool ad hoc fixes.
- Preserve existing failure contract and suppression semantics.

Rejected alternatives:

- Batch-wrapper payload mutation (for example `_batch`) was rejected because it changes downstream tool contracts.
- Strictly failing all non-`edit` concatenations was rejected because it does not satisfy the all-tools compatibility goal.

## Detailed Design

### 1. Stream Rewriter Responsibility

Introduce a stream-level stateful rewriter inside the existing from==to SSE forwarding path in `internal/runtime/executor/claude_executor.go`.

The rewriter tracks per-`index` tool block state:

- tool name/id
- accumulated input fragments
- suppression status after hard parse failure

### 2. Normalization Strategy

- `edit`:
  - Keep current path that delegates to `ToolCallNormalizer.NormalizeToolCall`.
  - Preserve existing path safety (`sanitizePatchPath`, operation checks) in `internal/runtime/executor/tool_call_normalizer.go`.
- non-`edit`:
  - Parse top-level object sequence with existing scanner logic (`splitTopLevelJSONObjectSequence`).
  - If sequence size is 0 or invalid: emit `invalid_tool_input`, suppress same-index follow-up content block events.
  - If sequence size is 1: passthrough.
  - If sequence size > 1: emit deterministic split calls as multiple valid tool blocks.

### 3. Event and Identity Rules

- Keep original event order for non-target blocks.
- For split output, each produced call must remain a valid `content_block_start -> content_block_delta -> content_block_stop` sequence.
- Preserve original index/id for the first split object where possible.
- Allocate deterministic unique indices and derived IDs for extra split objects.

### 4. Failure Contract

- Use existing parseable error shape: `type=error`, `error.type=invalid_tool_input`.
- Do not emit corrupted tool arguments.
- Suppress only the affected block stream until its stop event; keep unrelated blocks flowing.

### 5. Test Scope

Add and update tests in `internal/runtime/executor/claude_executor_test.go` and `internal/runtime/executor/tool_call_normalizer_test.go`:

- non-`edit` concatenated payload split behavior
- index/id determinism and no collisions
- invalid payload fail-loud + suppression isolation
- no regression for existing `edit -> apply_patch`
- no behavior changes for non-guarded paths (`from != to` or non-gemini model)

## Success Criteria

- Non-`edit` tools no longer fail from `{...}{...}` concatenation under the guarded path.
- Existing `edit` normalization behavior remains unchanged.
- Error events are parseable and block-local.
- Tests demonstrate deterministic splitting and non-regression.

## Design Documents

- [BDD Specifications](./bdd-specs.md) - Behavior scenarios and testing strategy
- [Architecture](./architecture.md) - System architecture and component details
- [Best Practices](./best-practices.md) - Security, performance, and code quality guidelines
