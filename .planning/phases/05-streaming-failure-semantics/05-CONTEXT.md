# Phase 5 Context: Streaming Failure Semantics

## Why This Phase Exists

Milestone v1 audit found a gap in the end-to-end correctness of metrics classification:

- Streaming terminal errors (especially OpenAI and Gemini) can emit an error payload to the client while the HTTP status code remains 200.
- RequestState has a LastError field, but it is not consistently set from terminal streaming error paths.
- The Query API splits success/failure using status_code and error_info, so misclassified failures corrupt percentiles and bucket aggregations.

This phase closes the gap by making streaming failure semantics unambiguous and persistable.

## Scope

- Ensure streaming terminal errors set an appropriate non-2xx HTTP status before writing the terminal SSE error payload.
- Ensure RequestState captures a non-empty error string on terminal streaming errors.
- Ensure persistence records carry failure signals (status_code and/or error_info) so Query API grouping is accurate.

## Out of Scope

- Any new API surface area.
- Any new feature flags or compatibility switches.
