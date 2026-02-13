# BDD Specifications for gemini tool_use normalization

## Feature: normalize concatenated tool_use payloads for all tools

This feature ensures that concatenated JSON object payloads are handled deterministically for all `tool_use` calls on the guarded path (`from == to` and `gemini-3-flash-preview`).

### Scenario 1: Preserve valid single-object non-edit tool input

**Given** a `tool_use` block for a non-`edit` tool

**And** the input is one valid JSON object

**When** stream normalization runs

**Then** the tool name and input are forwarded unchanged.

### Scenario 2: Split concatenated non-edit objects into independent calls

**Given** a `tool_use` block for a non-`edit` tool

**And** the input is a top-level object sequence like `{...}{...}{...}`

**When** stream normalization runs

**Then** the output contains multiple independent valid tool blocks

**And** each block contains exactly one object input

**And** original object order is preserved.

### Scenario 3: Keep edit conversion behavior unchanged

**Given** a `tool_use` block with tool name `edit`

**And** the input is concatenated edit objects

**When** normalization runs

**Then** the call is converted to `apply_patch`

**And** `patchText` includes all converted edits deterministically.

### Scenario 4: Reject malformed concatenated payloads with explicit error

**Given** a `tool_use` block with malformed concatenated JSON

**When** normalization runs

**Then** the stream emits a parseable `invalid_tool_input` error event

**And** no corrupted tool arguments are forwarded.

### Scenario 5: Suppress follow-up events only for the failed block

**Given** block index `n` failed normalization

**When** follow-up `content_block_delta` or `content_block_stop` events arrive for index `n`

**Then** those events are suppressed per existing failure contract

**And** other block indices continue streaming normally.

### Scenario 6: Preserve non-tool content events

**Given** the same response contains text/thinking/content events that are not target tool blocks

**When** normalization is active

**Then** those non-target events are forwarded without semantic changes.

### Scenario 7: Keep behavior isolated by model and route

**Given** requests where `from != to` or model is not `gemini-3-flash-preview`

**When** responses are streamed

**Then** this normalization and split logic does not execute.

## Testing Strategy

- **Unit tests**
  - object-sequence splitting edge cases (whitespace, escaped braces, malformed JSON)
  - deterministic index/id allocation for split events
  - existing `edit` conversion and path safety validations
- **Stream integration tests**
  - end-to-end SSE event sequence validity for split non-edit tool calls
  - fail-loud event emission and block-local suppression behavior
- **Regression tests**
  - existing `edit -> apply_patch` tests unchanged in intent
  - non-guarded paths remain behaviorally identical

## Definition of Done

- All scenarios above are covered by automated tests.
- No existing guarded-path success case regresses.
- New behavior is deterministic and parseable by downstream clients.
