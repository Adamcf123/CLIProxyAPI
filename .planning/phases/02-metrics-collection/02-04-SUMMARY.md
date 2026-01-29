# Phase 02 Plan 04: Gap Closure - Unified TTFT Sampling Summary

**Phase:** 02-metrics-collection
**Plan:** 04-gap-closure
**Subsystem:** metrics-collection
**Type:** execute
**Executed:** 2026-01-29
**Duration:** ~6 minutes

---

## One-Liner

Unified TTFT sampling across all streaming providers (OpenAI/Gemini/Claude) by ensuring first payload chunk goes through ForwardStream's write/flush path, with RequestState attached before any output.

---

## What Was Delivered

### Problem Solved

Phase 2 verification revealed that TTFT (Time To First Token) metrics were unreliable across providers because:

1. **OpenAI**: First chunk was written/flushed directly in handler before calling ForwardStream, bypassing `MaybeRecordFirstToken`
2. **Gemini**: RequestState was created and attached AFTER the first chunk was already written/flushed, making TTFT impossible to measure
3. **Claude**: Same issue as Gemini - state binding occurred too late

This caused TTFT to be either null or artificially inflated in the metrics summary.

### Solution Implemented

**Unified ForwardStream Path (Task 1)**
- Added `PrefetchedChunk` field to `StreamForwardOptions` struct
- Modified `ForwardStream` to write prefetched chunk before entering select loop
- Ensured `MaybeRecordFirstToken` is called for prefetched chunk
- Maintained consistency with existing keep-alive filtering logic

**OpenAI Handler (Task 2)**
- Modified `handleStreamingResponse` to pass prefetched chunk to `handleStreamResultWithPrefetched`
- Modified `handleCompletionsStreamingResponse` with same pattern
- Added `handleStreamResultWithPrefetched` helper function
- Removed direct write/flush of first chunk in handlers

**Gemini Handler (Task 3)**
- Moved `NewRequestState/AttachRequestState/StartLiveDisplay` BEFORE any write/flush
- Added `forwardGeminiStreamWithPrefetched` helper
- Prefetched chunk now passed via `StreamForwardOptions.PrefetchedChunk`
- Removed direct write/flush of first chunk

**Claude Handler (Task 4)**
- Same pattern as Gemini: state attachment moved before any output
- Added `forwardClaudeStreamWithPrefetched` helper
- Unified with other providers' sampling semantics

---

## Key Files

### Created
None

### Modified
- `sdk/api/handlers/stream_forwarder.go` - Added PrefetchedChunk support
- `sdk/api/handlers/openai/openai_handlers.go` - Unified first chunk path
- `sdk/api/handlers/gemini/gemini_handlers.go` - State attach before write
- `sdk/api/handlers/claude/code_handlers.go` - State attach before write

---

## Decisions Made

1. **PrefetchedChunk approach over MultiReader**: Chose to pass prefetched chunk explicitly via options rather than using `io.MultiReader` to maintain clear separation between "prefetch for inspection" and "forward for output"

2. **Helper function pattern**: Added `*WithPrefetched` variants of forward functions to maintain backward compatibility with existing non-prefetched call sites

3. **State attachment timing**: Moved state creation/attachment to immediately after successful first chunk receive but BEFORE headers are committed, ensuring TTFT captures true first token time

---

## Deviations from Plan

None - plan executed exactly as written.

---

## Verification

### Automated Tests
```bash
go test ./... -v
# All tests pass
```

### Manual Verification Required
Per plan requirements, manual verification needed:
1. Send streaming request to OpenAI endpoint
2. Send streaming request to Gemini endpoint
3. Send streaming request to Claude endpoint

**Expected**: stderr outputs single `metrics_summary {json}` line with non-null TTFT value when usage is available.

---

## Commits

| Commit | Message | Files |
|--------|---------|-------|
| 218ad79 | feat(02-04): add PrefetchedChunk support to ForwardStream | stream_forwarder.go |
| d09459f | feat(02-04): OpenAI streaming first chunk via ForwardStream | openai_handlers.go |
| 8d89424 | feat(02-04): Gemini streaming state attach before write | gemini_handlers.go |
| 713156c | feat(02-04): Claude streaming state attach before write | code_handlers.go |

---

## Next Phase Readiness

Phase 2 (Metrics Collection) is now complete with:
- ✅ Structured metrics logging (02-03)
- ✅ Live progress display (02-02)
- ✅ Request state tracking (02-01)
- ✅ Unified TTFT sampling across all providers (02-04)

All streaming providers now have consistent metrics semantics:
- TTFT measured at first actual payload chunk flush
- State attached before any output
- Single sampling path via ForwardStream

Ready for Phase 3 (Metrics Query/Historical).
