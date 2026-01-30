---
phase: 10-request-id-robustness
plan: 01
subsystem: infra
tags: [go, crypto-rand, request-id, hex]

# Dependency graph
requires:
  - phase: 03-persistence
    provides: metrics SQLite schema keyed by request_id
provides:
  - 64-bit (16-char hex) request_id generation while keeping API signature stable
  - Unit tests that lock request_id format, uniqueness, and concurrency safety
affects: [10-02-writer-conflict-detection, 10-03-conflict-tests, query-api]

# Tech tracking
tech-stack:
  added: []
  patterns: []

key-files:
  created: [internal/logging/requestid_test.go]
  modified: [internal/logging/requestid.go]

key-decisions:
  - "Kept crypto/rand as entropy source; only expanded byte length (32-bit -> 64-bit)"

patterns-established: []

# Metrics
duration: 1 min
completed: 2026-02-01
---

# Phase 10 Plan 01: Request ID Generator Upgrade Summary

**将 `GenerateRequestID()`（`internal/logging/requestid.go`）从 32-bit 升级为 64-bit 随机源，输出 16-char lowercase hex，显著降低 metrics 主键碰撞导致的“静默缺行”风险。**

## Performance

- **Duration:** 1 min
- **Started:** 2026-01-31T17:45:43Z
- **Completed:** 2026-01-31T17:47:01Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- `GenerateRequestID()` 现在生成 8 bytes 的随机数并编码为 16-char hex（64-bit 空间）。
- 保持向后兼容：`request_id` 仍为 TEXT 且查询逻辑不依赖固定长度，历史 8-char ID 仍可正常查询/匹配。
- 新增单元测试覆盖长度、字符集、概率性唯一性与并发安全，锁定新行为。

## Task Commits

Each task was committed atomically:

1. **Task 1: 升级 GenerateRequestID 到 64-bit** - `c5106d1` (feat)
2. **Task 2: 添加/更新单元测试** - `eb981f8` (test)

**Plan metadata:** (added after tasks)

## Files Created/Modified

- `internal/logging/requestid.go` - 生成 64-bit request_id（16-char hex）并更新 fallback。
- `internal/logging/requestid_test.go` - 锁定 request_id 格式/唯一性/并发安全的单元测试。

## Decisions Made

None - followed plan as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Ready for `10-request-id-robustness/10-02-PLAN.md` (writer 层冲突检测与可观测性)。

---
*Phase: 10-request-id-robustness*
*Completed: 2026-02-01*
