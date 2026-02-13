# Best Practices for gemini tool_use normalization

## Security Considerations

- Validate all reconstructed tool inputs at the boundary before forwarding.
- Preserve strict path and operation safety for `edit` via existing normalizer rules.
- Use fail-loud, parseable errors (`invalid_tool_input`) for malformed payloads.
- Keep error messages safe: no raw sensitive payload dumping.
- Isolate failures to one content block index; do not fail unrelated stream blocks.

## Performance Considerations

- Keep parsing linear to input size and avoid regex-based boundary detection.
- Reuse existing scanners for JSON object boundaries to reduce overhead.
- Keep per-index state bounded and release buffers immediately after block stop.
- Avoid unnecessary allocations in SSE rewrite path; clone only on emit.

## Reliability Considerations

- Preserve canonical guard to avoid broad blast radius.
- Keep deterministic ordering and identity generation for split outputs.
- Preserve protocol validity: each synthetic call must be a complete valid tool block sequence.
- Maintain existing suppression semantics to avoid repeated downstream failures after first parse error.

## Code Quality Guidelines

- Keep a single source of truth:
  - edit conversion and safety in `ToolCallNormalizer`
  - stream routing and splitting orchestration in executor
- Avoid per-tool custom branches except the explicit `edit` special case.
- Keep helper functions focused (state tracking, splitting, event assembly, error mapping).
- Add tests before implementation changes for split edge cases and regression guards.

## Testing and Verification

- Unit-test JSON sequence splitting with:
  - whitespace between objects
  - braces inside quoted strings
  - malformed/truncated inputs
- Integration-test SSE behavior with:
  - multi-object non-edit split
  - edit unchanged conversion path
  - block-local suppression after failure
- Regression-test non-guarded paths for no-op behavior.

## Common Pitfalls

- Treating each `input_json_delta` chunk as a complete JSON document.
- Expanding scope to all models/routes in one step.
- Emitting synthetic events with non-unique indices.
- Duplicating edit safety checks in multiple places.
- Relying on whole-log reads for validation instead of targeted tests and scoped inspection.
