---
phase: 08-persistence-contract-observability
plan: 01
subsystem: api
tags: [sqlite, metrics, observability, gin, atomic]

# Dependency graph
requires:
  - phase: 03-persistence
    provides: async SQLite writer and best-effort enqueue semantics
  - phase: 04-query-api
    provides: GET /v0/management/metrics response envelope + meta structure
provides:
  - Process-lifetime persistence drop health (quiet-period degraded + stable drop reasons)
  - Management metrics meta emits persistence health only when degraded
affects: [08-02, management-api, alerting]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Process-lifetime health via atomic counters + quiet period"
    - "Meta decoration emits only when degraded (omitempty pointer)"

key-files:
  created:
    - internal/metricspersist/health.go
    - internal/metricspersist/health_test.go
    - internal/api/handlers/management/metrics_persistence_test.go
  modified:
    - internal/metricspersist/writer.go
    - internal/metricspersist/writer_test.go
    - internal/api/handlers/management/metrics.go
    - internal/api/handlers/management/handler.go

key-decisions:
  - "Use a fixed 5m quiet period (code constant) to auto-clear degraded"
  - "Expose drop reasons as a stable enum set: queue_full, writer_not_started, insert_failure"
  - "Keep default /v0/management/metrics JSON unchanged by emitting meta.persistence only when degraded"

# Metrics
duration: 5 min
completed: 2026-01-30
---

# Phase 8 Plan 1: Persistence Contract & Observability Summary

**Best-effort 持久化的 drop/写入失败现在会形成进程级 degraded 信号，并且只在 degraded 时通过 /v0/management/metrics 的 meta.persistence 对管理员可见。**

## Performance

- **Duration:** 5 min
- **Started:** 2026-01-30T17:57:52Z
- **Completed:** 2026-01-30T18:03:32Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- 持久化健康源（`internal/metricspersist/health.go`）记录进程级 drop 总数、最近一次 drop 时间/原因，并用静默期计算 degraded。
- Writer 关键丢弃/失败路径（`internal/metricspersist/writer.go`）统一记录稳定原因码，且 Start 预检 Prepare 失败会 fail-fast（不再 goroutine 静默退出）。
- Management 查询（`internal/api/handlers/management/metrics.go`）仅在 degraded 时追加 `meta.persistence`，默认 JSON 输出不新增字段。

## Task Commits

Each task was committed atomically:

1. **Task 1: 在 metricspersist 建立单一 persistence health 数据源（drop 计数/原因/静默期恢复）并补齐关键 drop 点** - `17f7a37` (feat)
2. **Task 2: 在 /v0/management/metrics 的 meta 中仅在 degraded 时追加 meta.persistence（默认响应不变）** - `db6f30c` (feat)

## Files Created/Modified

- `internal/metricspersist/health.go` - 进程级持久化健康源（drop 计数 + last_drop + quiet period degraded）
- `internal/metricspersist/writer.go` - enqueue/prepare/exec 失败路径记录 drop reason；Start 预检 Prepare fail-fast
- `internal/api/handlers/management/metrics.go` - meta 增加 `persistence,omitempty` 并仅在 degraded 时输出
- `internal/api/handlers/management/handler.go` - Handler 构造时注入 persistence health provider（默认指向 metricspersist）
- `internal/metricspersist/health_test.go` - health 静默期与 drop 点回归测试
- `internal/api/handlers/management/metrics_persistence_test.go` - 合同测试锁定默认 JSON 不漂移、degraded 时最小字段集合

## Decisions Made

- 使用固定 `5m` 静默期常量计算 degraded（不引入新配置项），保证运行中可自动恢复。
- drop 原因对外稳定为 `queue_full` / `writer_not_started` / `insert_failure` 三类，避免泄露 request_id/SQL/error 原文。
- `meta.persistence` 仅在 degraded 输出（指针 + `omitempty`），保证默认 JSON 结构保持不变。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Removed t.Parallel from async writer test to avoid global singleton interference**
- **Found during:** Task 1
- **Issue:** metricspersist writer/health 是进程级单例；并行测试会让 drop 计数类断言/状态串扰变得不稳定
- **Fix:** 取消 `internal/metricspersist/writer_test.go` 的并行标记，确保包内测试稳定
- **Files modified:** internal/metricspersist/writer_test.go
- **Verification:** go test ./internal/metricspersist
- **Committed in:** 17f7a37

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** 修复是为保证回归测试稳定性；不改变运行时语义，无范围扩张。

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 08 的运行时可观测性信号已具备，准备进入 `08-02-PLAN.md` 完成契约文档与更多契约测试。

---
*Phase: 08-persistence-contract-observability*
*Completed: 2026-01-30*
