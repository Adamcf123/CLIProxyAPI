---
phase: 03-persistence
plan: 03
subsystem: database
tags: [sqlite, goose, retention, metrics]

# Dependency graph
requires:
  - phase: 03-persistence
    provides: Async SQLite writer + MetricsPlugin integration (03-02)
provides:
  - SQLite metrics retention cleanup (keep last 7 days)
  - Legacy JSONL metrics logging removal (internal/metricslog decommissioned)
affects: [04-query-api]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Fail-fast migrations plus startup retention enforcement
    - Best-effort background maintenance in the SQLite writer goroutine

key-files:
  created:
    - internal/metricspersist/cleanup_test.go
  modified:
    - internal/metricspersist/db.go
    - internal/metricspersist/migrations.go
    - internal/metricspersist/writer.go
    - internal/metricsruntime/usage_plugin.go

key-decisions:
  - "Retention cleanup runs after migrations and on a 24h ticker inside the SQLite writer to bound DB growth for long-lived processes."
  - "Decommissioned internal/metricslog and removed JSONL emission from MetricsPlugin so SQLite is the single source of truth for metrics."

patterns-established:
  - "Retention maintenance: DELETE based on created_at < datetime('now', '-N days') using a parameterized modifier."

# Metrics
duration: 5m30s
completed: 2026-01-30
---

# Phase 3 Plan 03: Retention Cleanup + Disable JSONL Legacy Summary

**SQLite 自动清理 7 天前的 metrics 行，并完全移除遗留 JSONL 写盘路径（internal/metricslog）**

## Performance

- **Duration:** 5m30s
- **Started:** 2026-01-30T07:33:25Z
- **Completed:** 2026-01-30T07:38:55Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- 指标持久化（`internal/metricspersist/db.go`）新增保留期清理能力 → 数据库只保留最近 7 天数据
- 启动流程（`internal/metricspersist/migrations.go`）在迁移后执行一次清理 → 冷启动时立刻收敛历史数据
- 遗留 JSONL 日志链路（`internal/metricsruntime/usage_plugin.go` → `internal/metricslog`）移除 → SQLite 成为 metrics 单一数据源

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement Retention Cleanup** - `73dcfe3` (feat)
2. **Task 2: Disable JSONL and Clean up** - `e26698c` (feat)

## Files Created/Modified
- `internal/metricspersist/db.go` - 新增 `Cleanup(db, retentionDays)`，按 created_at 执行保留期删除
- `internal/metricspersist/migrations.go` - 迁移完成后执行一次保留期清理
- `internal/metricspersist/writer.go` - 写入协程内加入 24h 定时清理，避免长期运行导致 DB 增长
- `internal/metricspersist/cleanup_test.go` - 覆盖“10 天前数据会被删除、1 天内数据保留”的行为测试
- `internal/metricsruntime/usage_plugin.go` - 移除 JSONL enqueue，直接构造 SQLite 写入记录

## Decisions Made
- 将 retention enforcement 放在迁移后与 writer 定时任务中：启动时强制收敛、运行中持续维持 7 天窗口。
- 彻底下线 JSONL：删除 `internal/metricslog`，避免双写与数据源分裂，SQLite 成为唯一事实来源。

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Retention 与单数据源目标已满足，Phase 4 可以直接基于 `logs/metrics.db` 做查询与聚合 API。
- 现有历史 `logs/metrics-*.jsonl` 文件不会被新版本继续写入；如需回收磁盘空间可在运维侧单独清理。

---
*Phase: 03-persistence*
*Completed: 2026-01-30*
