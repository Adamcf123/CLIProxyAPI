---
phase: 06-guaranteed-usage-publish
plan: 01
subsystem: testing
tags: [go, usage, sync.Once, unit-test]

# Dependency graph
requires:
  - phase: 05-streaming-failure-semantics
    provides: "streaming failure persistence semantics (failure remains authoritative)"
provides:
  - "usageReporter publish seam (publishUsageRecord) for isolated unit tests"
  - "usageReporter finalize() helper to enforce failure-first then ensurePublished ordering"
  - "unit tests that lock no-usage publish + failure precedence + once semantics"
affects: [06-guaranteed-usage-publish, executor, sqlite-persistence]

# Tech tracking
tech-stack:
  added: []
  patterns: ["package-level function seam for publish side effects", "failure-first finalize helper for non-streaming code paths"]

key-files:
  created: [internal/runtime/executor/usage_helpers_test.go]
  modified: [internal/runtime/executor/usage_helpers.go]

key-decisions:
  - "None - followed plan as specified"

patterns-established:
  - "publishUsageRecord seam: override in tests, restore via defer"
  - "finalize(ctx, &err) ordering: trackFailure then ensurePublished"

# Metrics
duration: 2 min
completed: 2026-01-30
---

# Phase 6 Plan 01: Guaranteed Usage Publish Summary

**usageReporter 通过 publishUsageRecord 测试缝 + finalize() 收敛路径，并用单元测试锁定 no-usage 可发布、失败优先级与 once 语义**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-30T15:06:29Z
- **Completed:** 2026-01-30T15:09:02Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- 侧效应发布（`internal/runtime/executor/usage_helpers.go`）增加可替换的 publish seam → 单测可同步捕获发布，不依赖全局 usage manager
- 收敛非流式 finalize 帮助函数（`internal/runtime/executor/usage_helpers.go`）→ 明确“先失败后兜底 ensurePublished”的顺序，避免 defer 顺序踩坑
- 增加 usageReporter 单测（`internal/runtime/executor/usage_helpers_test.go`）→ 锁定 no-usage success 仍可 ensurePublished 发布、failure 不可被覆盖、once 至多发布一次

## Task Commits

Each task was committed atomically:

1. **Task 1: Add deterministic seams + a safe finalize helper** - `4ce4bc1` (feat)
2. **Task 2: Add unit tests for no-usage and failure precedence** - `b7c7fbf` (test)

## Files Created/Modified

- `internal/runtime/executor/usage_helpers.go` - 引入 publishUsageRecord seam，并新增 finalize() 收敛失败优先 + ensurePublished 顺序
- `internal/runtime/executor/usage_helpers_test.go` - 新增 6 个单测，锁定 no-usage / failure precedence / once 语义

## Decisions Made

None - followed plan as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Ready for `06-02-PLAN.md` (streaming executors call ensurePublished at end-of-stream goroutine).

---
*Phase: 06-guaranteed-usage-publish*
*Completed: 2026-01-30*
