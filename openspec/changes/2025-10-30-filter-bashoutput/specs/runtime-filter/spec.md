# Capability: Runtime BashOutput Filtering

## ADDED Requirements

### Requirement: Server-side filtering for invalid BashOutput
- The system MUST filter out BashOutput tool calls that do not reference a valid server-recognized shell session.
- The system MUST NOT forward such tool calls to downstream clients (Claude Code).
- The system MUST apply filtering in both request conversion (Claude→Codex) and response conversion (Codex→Claude) paths.
- The system MUST preserve normal behavior for other tools.

#### Scenario: Streaming response filtering
- Given a Codex streaming response with `response.output_item.added` where `item.name` is BashOutput and arguments missing/invalid `bash_id`
- When converting to Claude SSE
- Then the converter MUST NOT emit any `tool_use` events for that item, and MUST skip subsequent `arguments.delta` / `output_item.done` for the same index.

#### Scenario: Non-streaming response filtering
- Given a Codex non-streaming response with an `output` element of type `function_call` where `name` is BashOutput and `arguments` missing/invalid `bash_id`
- When converting to Claude message JSON
- Then the converter MUST NOT include any `tool_use` block for that element.

#### Scenario: Request-side filtering
- Given a Claude request containing a `tool_use` whose `name` is BashOutput and input lacks valid `bash_id`
- When converting to Codex input `function_call`
- Then the converter MUST NOT append such function_call into the `input` array.

### Requirement: Stop reason consistency
- The system MUST set `stop_reason` to `tool_use` only if there exists at least one valid (non-filtered) tool call in the turn.
- Otherwise, `stop_reason` MUST be `end_turn`.

#### Scenario: No valid tool calls
- Given a turn where all tool calls are filtered (e.g., BashOutput invalid `bash_id`)
- When generating the completion summary
- Then the system MUST set `stop_reason` to `end_turn`.

## MODIFIED Requirements

### Requirement: Robust tool name / argument handling
- The system MUST treat tool names case-insensitively for BashOutput matching.
- The system MUST validate `arguments` as JSON and require a plausible `bash_id`.
- The system MAY refine `bash_id` format validation without breaking clients.

#### Scenario: Case-insensitive name match
- Given tool name variations like `BashOutput`, `bashoutput`, or `BASHOUTPUT`
- When applying filtering
- Then the system MUST treat them equivalently.

#### Scenario: Arguments validation
- Given arguments that are not valid JSON or missing `bash_id`
- When applying filtering
- Then the system MUST drop the tool call.
