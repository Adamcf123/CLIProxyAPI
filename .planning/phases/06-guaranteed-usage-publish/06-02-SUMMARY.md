---
phase: 06-guaranteed-usage-publish
plan: 02
subsystem: infra
tags: [go, usage, sqlite, streaming, executor, claude, codex, qwen]

# Dependency graph
requires:
  - phase: 06-guaranteed-usage-publish/06-01
    provides: usageReporter.finalize/ensurePublished semantics + tests
provides:
  - Claude/Codex/Qwen executors always emit at least one usage record per request (even when tokens are missing)
  - Streaming paths defer ensurePublished at stream end inside the scanner goroutine
affects: [06-03, 06-04, 06-05, metrics-query]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Non-streaming Execute() defers usageReporter.finalize(ctx, &err) to avoid defer-order hazards
    - Streaming ExecuteStream() defers usageReporter.ensurePublished(ctx) inside the stream goroutine

key-files:
  created: []
  modified:
    - internal/runtime/executor/claude_executor.go
    - internal/runtime/executor/codex_executor.go
    - internal/runtime/executor/qwen_executor.go

key-decisions:
  - "None - followed plan as specified"

patterns-established:
  - "Executor end-of-request hook: non-streaming uses reporter.finalize; streaming uses defer reporter.ensurePublished inside the stream goroutine"

# Metrics
duration: 2 min
completed: 2026-01-30
---

# Phase 6 Plan 02: Claude/Codex/Qwen EnsurePublished Summary

**Claude/Codex/Qwen 执行器在 usage/tokens 缺失时也能保证至少发布 1 条 usage record，从而 SQLite 始终可查询到该请求。**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-30T15:12:15Z
- **Completed:** 2026-01-30T15:15:02Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- 统一非流式收尾逻辑（`internal/runtime/executor/*_executor.go`）使用 `reporter.finalize(ctx, &err)` → 成功但无 usage 的请求也会被 ensurePublished 覆盖落库
- 统一流式收尾逻辑（`ExecuteStream` goroutine）`defer reporter.ensurePublished(ctx)` → 即使没有任何 usage chunk，stream 结束也会补发 1 条 usage record
- 保持失败语义不变：错误路径仍通过 `publishFailure`/`finalize` 先发布 failure，ensurePublished 不会覆盖 failure

## Task Commits

Each task was committed atomically:

1. **Task 1: Non-streaming paths use reporter.finalize (no defer-order hazards)** - `81a6118` (fix)
2. **Task 2: Streaming goroutines ensure publish at stream end** - `4a7ab1a` (fix)

## Files Created/Modified
- `internal/runtime/executor/claude_executor.go` - 非流式改用 finalize；流式 goroutine 结束时 ensurePublished
- `internal/runtime/executor/codex_executor.go` - 非流式改用 finalize；流式 goroutine 结束时 ensurePublished
- `internal/runtime/executor/qwen_executor.go` - 非流式改用 finalize；流式 goroutine 结束时 ensurePublished

## Decisions Made
None - followed plan as specified.

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
- Task 2 提交时工作区存在预先 staged 的其它 executor 文件，导致 commit 误包含无关文件；已用 `b04a471` 将这些文件恢复到任务前状态，确保 06-02 只影响 Claude/Codex/Qwen。

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Ready for `06-03-PLAN.md`（Gemini/Vertex/AIStudio 执行器补齐 ensurePublished 收尾）。

---
*Phase: 06-guaranteed-usage-publish*
*Completed: 2026-01-30*
