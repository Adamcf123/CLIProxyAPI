---
phase: 05-streaming-failure-semantics
plan: 02
subsystem: api
tags: [go, gin, streaming, sse, openai, gemini]

# Dependency graph
requires:
  - phase: 04-query-api
    provides: Query API classifies success/failure via status_code + error_info
provides:
  - Best-effort HTTP status semantics for OpenAI/Gemini streaming terminal errors (when headers not yet committed)
affects: [05-streaming-failure-semantics, query-api, metrics-classification]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Streaming terminal error writers call c.Status(status) before writing terminal payload

key-files:
  created:
    - sdk/api/handlers/openai/openai_handlers_test.go
    - sdk/api/handlers/gemini/gemini_handlers_test.go
  modified:
    - sdk/api/handlers/openai/openai_handlers.go
    - sdk/api/handlers/openai/openai_responses_handlers.go
    - sdk/api/handlers/gemini/gemini_handlers.go
    - sdk/api/handlers/gemini/gemini-cli_handlers.go

key-decisions:
  - "None - followed plan as specified"

patterns-established:
  - "Best-effort status: set HTTP status before writing terminal streaming error payload; rely on error_info when headers already committed"

# Metrics
duration: 4 min
completed: 2026-01-30
---

# Phase 5 Plan 2: Streaming Failure Semantics Summary

**OpenAI/Gemini streaming terminal error 在写出终止错误 payload 前显式设置 HTTP status（headers 未提交时生效）**

## Performance

- **Duration:** 4 min
- **Started:** 2026-01-30T10:49:27Z
- **Completed:** 2026-01-30T10:54:26Z
- **Tasks:** 3
- **Files modified:** 6

## Accomplishments

- OpenAI（`sdk/api/handlers/openai/openai_handlers.go` / `sdk/api/handlers/openai/openai_responses_handlers.go`）的 `WriteTerminalError` 在写 SSE error payload 前调用 `c.Status(status)`，对齐 Claude 行为。
- Gemini SSE 与 alt（raw JSON）两分支（`sdk/api/handlers/gemini/gemini_handlers.go` / `sdk/api/handlers/gemini/gemini-cli_handlers.go`）在写终止错误 payload 前调用 `c.Status(status)`。
- 新增最小回归测试锁定“未写入任何 chunk 就 terminal error”时响应 status 会变为非 2xx（OpenAI + Gemini 各 1）。

## Task Commits

Each task was committed atomically:

1. **Task 1: OpenAI streaming terminal error 在写 payload 前调用 c.Status(status)** - `5ac36bf` (fix)
2. **Task 2: Gemini streaming terminal error 在写 payload 前调用 c.Status(status)** - `ea93230` (fix)
3. **Task 3: 新增回归测试：未写入任何 chunk 即 terminal error 时 status 会变为非 2xx** - `3fb9725` (test)

## Files Created/Modified

- `sdk/api/handlers/openai/openai_handlers.go` - OpenAI chat completions streaming terminal error 在写 `data: {error...}` 前设置 status。
- `sdk/api/handlers/openai/openai_responses_handlers.go` - OpenAI Responses SSE terminal error 在写 `event: error`/`data: {error...}` 前设置 status。
- `sdk/api/handlers/gemini/gemini_handlers.go` - Gemini SSE/alt terminal error 在写 payload 前设置 status。
- `sdk/api/handlers/gemini/gemini-cli_handlers.go` - Gemini CLI SSE/alt terminal error 在写 payload 前设置 status。
- `sdk/api/handlers/openai/openai_handlers_test.go` - 回归测试：无 chunk 情况下 terminal error 能把 status 设为 401。
- `sdk/api/handlers/gemini/gemini_handlers_test.go` - 回归测试：无 chunk 情况下 terminal error 能把 status 设为 500。

## Decisions Made

None - followed plan as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 已提供 best-effort 的 status 语义（headers 未提交时生效）；headers 已 commit 场景仍依赖 05-01 的 `error_info` 作为强保证。
- Ready for继续执行：`05-01-PLAN.md`（写入可持久化 failure 信号）与 `05-03-PLAN.md`（Query API failure 归类回归测试）。

---
*Phase: 05-streaming-failure-semantics*
*Completed: 2026-01-30*
