# Architecture for gemini tool_use normalization

## System Overview

The change stays in the current streaming execution path where SSE lines are forwarded for `from == to` requests in `internal/runtime/executor/claude_executor.go`.

The architecture adds one canonical stream rewrite capability:

- keep `edit` specialization through the existing `ToolCallNormalizer`
- add generic non-`edit` split support for concatenated top-level object sequences

No translator-side protocol redesign is introduced.

## Components

### 1) Stream Normalization State (executor-local)

Location: `internal/runtime/executor/claude_executor.go`

Responsibilities:

- track per-index tool block metadata
- preserve suppression state for failed blocks
- detect target tool events and route to the appropriate strategy

State model:

- `suppressed[index]` for failed block suppression until stop
- block context `{index, toolName, toolID, inputFragments}`

### 2) Edit Strategy (existing)

Location: `internal/runtime/executor/tool_call_normalizer.go`

Responsibilities:

- convert concatenated `edit` payloads into one `apply_patch` payload
- validate path safety and allowed operation types

Invariants maintained:

- deterministic file order in generated patch
- fail-fast on unsafe/ambiguous edit objects

### 3) Generic Object Sequence Splitter (new behavior)

Primary logic can reuse parser utilities already present in executor/normalizer code.

Responsibilities:

- for non-`edit` tool input, detect top-level JSON object sequence (`{...}{...}`)
- split into N independent valid calls when N > 1
- preserve object order and deterministic generated identities

## Data Structures

Suggested internal structures in executor scope:

- `toolBlockState`
  - `index int64`
  - `name string`
  - `id string`
  - `accumulatedInput bytes.Buffer`
- `normalizationState`
  - `suppressed map[int64]struct{}`
  - `active map[int64]toolBlockState`
  - `maxObservedIndex int64`

## Integration Points

1. `ExecuteStream` in `internal/runtime/executor/claude_executor.go`
   - maintain current guard: `from == to && baseModel == "gemini-3-flash-preview"`
   - call unified normalization function that may emit one or multiple output lines

2. `normalizeToolUseLine` path in `internal/runtime/executor/claude_executor.go`
   - keep existing error event and suppression semantics
   - extend handling from edit-only payload rewrite to generic split orchestration

3. `ToolCallNormalizer` in `internal/runtime/executor/tool_call_normalizer.go`
   - remain the single source of truth for `edit` conversion
   - avoid duplicating edit-specific validation rules in executor

4. Tests
   - update `internal/runtime/executor/claude_executor_test.go`
   - extend `internal/runtime/executor/tool_call_normalizer_test.go` only where relevant

## Event Rewrite Rules

1. Non-target event: passthrough.
2. Target `edit` event: apply existing conversion logic.
3. Target non-`edit` with one object: passthrough.
4. Target non-`edit` with multiple objects:
   - emit first object in original block context
   - emit additional synthetic valid tool blocks for remaining objects
5. Parse failure:
   - emit `invalid_tool_input`
   - suppress same-index block follow-up content events until stop

## Technology Choices

- Keep using existing JSON scanning approach already in executor/normalizer.
- Keep existing SSE forwarding model and event format.
- Do not add external dependency for parsing/splitting.

Rationale:

- minimal change surface on hot path
- preserves existing contracts
- easier regression control with current test harness

## Risks and Mitigations

- **Risk:** event order or index collision during synthetic split emission
  - **Mitigation:** deterministic index allocator and sequence assertions in tests
- **Risk:** accidental behavior changes outside guarded model/route
  - **Mitigation:** preserve guard and add explicit non-guarded regression tests
- **Risk:** malformed payloads leaking partial sensitive content in errors
  - **Mitigation:** keep concise parseable error type, avoid echoing raw payloads
