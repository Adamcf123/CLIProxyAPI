# Phase 5 Research: Streaming Failure Semantics

## Problem Restatement (from v1 audit)

We need streaming terminal errors to be unambiguous and persistable:

- Some streaming paths can emit a terminal error payload while the HTTP status remains 200.
- `RequestState.LastError` exists but is not consistently set for terminal streaming errors.
- Query API splits success/failure using `status_code` + `error_info`; misclassification pollutes percentiles/buckets.

## Existing Code Paths (where to fix)

1) Stream forwarding fan-in

- The shared stream loop lives at `sdk/api/handlers/stream_forwarder.go`.
- It already sees terminal errors via `errs <-chan *interfaces.ErrorMessage`.
- It already has access to the Gin context and `metricsruntime`.

Conclusion: this is the best single place to set a persistable failure signal (`RequestState.LastError`) for all streaming providers.

2) Provider-specific terminal error payload writers

- Provider handlers format the terminal error payload in closures passed as `StreamForwardOptions.WriteTerminalError`.
- Claude already calls `c.Status(status)` in its terminal error writer (`sdk/api/handlers/claude/code_handlers.go`).
- OpenAI and Gemini variants currently build an error payload but do not set HTTP status.

Conclusion: align OpenAI/Gemini terminal error writers with Claude by calling `c.Status(status)` before writing the terminal payload.

## Important Constraints / Pitfalls

- In streaming responses, once any bytes are flushed, the HTTP status may already be committed.
  - Some paths write a prefetched chunk and flush before entering the streaming loop.
  - Therefore, the HTTP status cannot be relied upon as the only failure signal.

Conclusion: we must rely on a persisted `error_info` signal (via `RequestState.LastError`) to classify failures even when the status is stuck at 200.

## Testing Strategy

- Add a focused unit test for `ForwardStream` to ensure that any terminal error sets `RequestState.LastError` (including when `ErrorMessage.Error` is nil, via a fallback string).
- Strengthen Query API tests (buckets mode) to cover the case `status_code=200` + `error_info != ''` => failure.
