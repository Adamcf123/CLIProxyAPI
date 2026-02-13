# Design: gemini edit-to-apply-patch normalization

## Goal

Normalize malformed `edit` tool payloads emitted by `gemini-3-flash-preview` when `from == to` so concatenated JSON objects are converted into one valid `apply_patch` call that can include multiple files and multiple hunks.

## Scope

- In scope: streaming assistant tool calls where tool name is `edit`, model is `gemini-3-flash-preview`, and payload is concatenated JSON objects.
- In scope: deterministic conversion to a single `apply_patch` input with complete change coverage.
- In scope: safe fallback when conversion is not reliable.
- Out of scope: changing behavior for other models, non-`edit` tools, or already valid single-object tool inputs.

## Constraints

- Keep output protocol-compatible with current SSE forwarding.
- Preserve event order and stop reasons.
- Conversion must be lossless for supported patterns; if not possible, fail fast with explicit error semantics.
- No network dependency in tests; use test doubles for upstream stream and tool events.

## Architecture Boundaries

- Detection and normalization occur in the `from == to` streaming path before chunk forwarding.
- Parsing strategy must separate multiple top-level JSON objects from one input stream fragment.
- Mapping strategy must merge multiple `edit` objects into one `apply_patch` object with a single `patchText`.
- Safety strategy must reject ambiguous or unsupported object shapes.

## BDD Source

- Source of truth: `./bdd-specs.md`
