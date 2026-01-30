---
phase: 03-persistence
plan: 02
subsystem: database
tags: [sqlite, database-sql, goroutine, async-writer, metrics]

# Dependency graph
requires:
  - phase: 03-persistence/03-01
    provides: SQLite schema migrations + DB init wiring (logs/metrics.db)
provides:
  - Async SQLite writer (buffered, non-blocking enqueue)
  - MetricsPlugin -> SQLite persistence flow keyed by request_id
affects: [03-persistence/03-03, 04-history-queries]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Non-blocking enqueue + single background writer goroutine for persistence

key-files:
  created:
    - internal/metricspersist/types.go
    - internal/metricspersist/writer.go
    - internal/metricspersist/writer_test.go
  modified:
    - internal/metricsruntime/usage_plugin.go
    - cmd/server/main.go

key-decisions:
  - "SQLite metrics rows use request_id as the de-duplication key and never store request_path"
  - "Writer drops records on queue-full (and before startup) to guarantee zero request-path latency impact"

patterns-established:
  - "Async persistence boundary: plugin enqueues record, writer owns all DB inserts"

# Metrics
duration: 5min
completed: 2026-01-30
---

# Phase 3 Plan 2: Async SQLite Writer + MetricsPlugin Integration Summary

**将每次请求的指标写入从请求路径剥离为后台 SQLite writer（ON CONFLICT(request_id) 去重），并在 MetricsPlugin 中把 usage 数据异步落到 logs/metrics.db（不存 request_path）**

## Performance

- **Duration:** 5 min
- **Started:** 2026-01-30T07:25:58Z
- **Completed:** 2026-01-30T07:30:55Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- 指标持久化（internal/metricspersist）新增 `MetricRecord` + buffered queue + goroutine writer，确保 enqueue 永不阻塞请求
- 去重策略（internal/metricspersist/writer.go）使用 `ON CONFLICT(request_id) DO NOTHING`，同一 request_id 幂等写入
- 插件集成（internal/metricsruntime/usage_plugin.go）从 ctx 提取 `request_id` 并异步写入 SQLite，同时保留现有 JSONL enqueue 作为过渡

## Task Commits

每个任务均原子提交：

1. **Task 1: Implement Async Writer** - `64366e5` (feat)
2. **Task 2: Integrate MetricsPlugin** - `8bf652f` (feat)

## Files Created/Modified
- `internal/metricspersist/types.go` - SQLite 行级指标的内部结构体（排除 request_path）
- `internal/metricspersist/writer.go` - 默认 writer（buffered queue）+ `StartWriter`/`Enqueue`，后台 goroutine 负责 INSERT
- `internal/metricspersist/writer_test.go` - 临时 SQLite DB 上验证异步写入与 request_id 去重
- `internal/metricsruntime/usage_plugin.go` - 从 `logging.GetRequestID(ctx)` 构建 `metricspersist.MetricRecord` 并 enqueue
- `cmd/server/main.go` - migrations 后启动 SQLite writer（确保生产路径实际落库）

## Decisions Made
- 使用与 schema 一致的 `MetricRecord`（internal/metricspersist/types.go）作为 DB 写入边界对象；敏感字段 request_path 不进入 DB
- 将“写入失败/队列满”的处理策略固定为 fail-silent + drop（internal/metricspersist/writer.go），以保障请求延迟不受 DB I/O 影响

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] 在服务启动路径显式调用 StartWriter，避免 Enqueue 无消费者导致永不落库**
- **Found during:** Task 2 (Integrate MetricsPlugin)
- **Issue:** 计划文件未包含 writer 启动点；仅修改 plugin 会导致数据 enqueue 后无人消费
- **Fix:** 在 `cmd/server/main.go` 的 migrations 之后调用 `metricspersist.StartWriter(db)` 并失败即退出
- **Files modified:** cmd/server/main.go
- **Verification:** `go test ./...` 通过；writer 单测验证实际写入 SQLite
- **Committed in:** 8bf652f

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** 偏差仅补齐必需的启动 wiring，无新增需求/无范围膨胀。

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- SQLite writer + plugin enqueue 链路已建立，可在 03-03 里移除 JSONL 单一来源并补齐保留策略（7 天）

---
*Phase: 03-persistence*
*Completed: 2026-01-30*
