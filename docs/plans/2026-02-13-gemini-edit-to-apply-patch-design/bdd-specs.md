# BDD Specs: gemini edit-to-apply-patch normalization

## Feature

Convert malformed concatenated `edit` tool payloads into one valid `apply_patch` payload for `gemini-3-flash-preview` on `from == to` streaming path.

## Scenario 1: Convert concatenated edit objects

**Given** an assistant `tool_use` event for tool `edit` from model `gemini-3-flash-preview` on `from == to`

**And** the tool input contains multiple top-level JSON objects concatenated as `{...}{...}{...}`

**When** the proxy normalizes the tool payload

**Then** the proxy emits a single valid tool input object for `apply_patch`

**And** the generated `patchText` includes all edits from all parsed objects.

## Scenario 2: Preserve valid single-object edit payloads

**Given** an assistant `tool_use` event for tool `edit` with one valid JSON object input

**When** the proxy evaluates normalization

**Then** the payload is forwarded without conversion

**And** tool name and arguments remain unchanged.

## Scenario 3: Fail safely on ambiguous payloads

**Given** an assistant `tool_use` event for tool `edit` where concatenated objects cannot be safely mapped to patch operations

**When** normalization is attempted

**Then** the proxy does not emit corrupted tool arguments

**And** it emits an explicit, parseable error outcome for the client.

## Scenario 4: Support more than three files and hunks

**Given** concatenated edit objects that target more than three distinct files and multiple replacements per file

**When** conversion runs

**Then** one `apply_patch` payload is produced

**And** every file operation appears in deterministic order in `patchText`.

## Scenario 5: Keep behavior isolated by model and route

**Given** requests not matching both conditions (`model != gemini-3-flash-preview` or `from != to`)

**When** tool payloads are streamed

**Then** no edit-to-apply_patch conversion is applied.
