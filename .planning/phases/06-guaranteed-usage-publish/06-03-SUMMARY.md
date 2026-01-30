---
phase: 06-guaranteed-usage-publish
plan: 03
subsystem: api
tags: [go, executor, gemini, vertex, aistudio, sqlite, usage]

# Dependency graph
requires:
  - phase: 06-guaranteed-usage-publish/06-01
    provides: usageReporter.ensurePublished semantics + test seam
provides:
  - Gemini/Vertex/AIStudio non-streaming requests finalize via usageReporter.finalize
  - Gemini/Vertex/AIStudio streaming goroutines defer usageReporter.ensurePublished at stream end
affects: [06-04, 06-05, metrics-persistence]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Non-streaming executors use usageReporter.finalize(ctx, &err) as the canonical end-of-request hook"
    - "Streaming executors defer usageReporter.ensurePublished(ctx) inside the stream goroutine"

key-files:
  created: []
  modified:
    - internal/runtime/executor/gemini_executor.go
    - internal/runtime/executor/gemini_cli_executor.go
    - internal/runtime/executor/gemini_vertex_executor.go
    - internal/runtime/executor/aistudio_executor.go

key-decisions:
  - "None - followed plan as specified"

patterns-established:
  - "Guarantee at least one usage record per request without weakening failure semantics"

# Metrics
duration: 6 min
completed: 2026-01-30
---

# Phase 6 Plan 03: Guaranteed Usage Publish Summary

**Gemini/Vertex/AIStudio 在 usage/tokens 缺失时也会落至少 1 条 usage 记录，并在流式结束时强制 ensurePublished（不覆盖失败语义）**

## Performance

- **Duration:** 6 min
- **Started:** 2026-01-30T15:12:56Z
- **Completed:** 2026-01-30T15:19:02Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- 非流式 Gemini/Vertex/AIStudio 执行路径（各自 executor 的 Execute/executeWith*）统一用 `reporter.finalize(ctx, &err)` 作为请求收尾钩子 → 成功但无 usage 也会 ensurePublished
- 流式 Gemini/Vertex/AIStudio 执行路径在 streaming goroutine 内 `defer reporter.ensurePublished(ctx)` → 即使 stream 中从未出现 usage chunk，也能在流末发布 1 条记录
- 失败语义保持 fail-first：既有 `publishFailure(ctx)` 路径不变，ensurePublished 只在“未发布任何记录”时兜底

## Task Commits

Each task was committed atomically:

1. **Task 1: Non-streaming paths use reporter.finalize (no defer-order hazards)** - `e7b6762` (fix)
2. **Task 2: Streaming goroutines ensure publish at stream end** - `f20dd1e` (fix)

**Plan metadata:** (recorded in the docs(06-03) plan-completion commit)

## Files Created/Modified

- `internal/runtime/executor/gemini_executor.go` - 流式 goroutine 增加 ensurePublished；非流式用 finalize 收尾
- `internal/runtime/executor/gemini_cli_executor.go` - 流式 goroutine 增加 ensurePublished；非流式用 finalize 收尾
- `internal/runtime/executor/gemini_vertex_executor.go` - 两条流式 goroutine 增加 ensurePublished；两条非流式 helper 用 finalize 收尾
- `internal/runtime/executor/aistudio_executor.go` - 流式 goroutine 增加 ensurePublished；非流式用 finalize 收尾

## Decisions Made

None - followed plan as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- 无

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 已满足本计划 must-haves：Gemini/Vertex/AIStudio 在无 usage 场景也会发布记录；流式在 stream end 兜底 ensurePublished；失败语义不被覆盖
- 可继续执行：06-04（SQLite 回归测试）与 06-05（stream end ensurePublished wiring 回归）

---
*Phase: 06-guaranteed-usage-publish*
*Completed: 2026-01-30*
