---
phase: 02-metrics-collection
plan: 03
subsystem: api
tags: [go, metrics, jsonl, logging, tps, ttft, tpot]

# Dependency graph
requires:
  - phase: 02-metrics-collection-01
    provides: RequestState for per-request metric sampling and TPSCollector for aggregation
  - phase: 01-metrics-foundation
    provides: TPSCollector with sliding window and metric calculation primitives
provides:
  - Async daily-rotated JSONL writer for structured metrics logs
  - MetricsPlugin subscribing to usage.Record and computing TPS/TTFT/TPOT
  - Automatic registration via internal/usage init (no blank imports)
affects: [02-metrics-collection, logging, persistence, observability]

# Tech tracking
tech-stack:
  added: []
  patterns: ["Async non-blocking log enqueue with queue-full drop", "Daily log rotation with date-based filename", "Plugin model for usage record processing"]

key-files:
  created:
    - internal/metricslog/types.go
    - internal/metricslog/jsonl_writer.go
    - internal/metricsruntime/usage_plugin.go
    - internal/usage/metrics_plugin.go
  modified: []

key-decisions:
  - "Use pointer fields in MetricsLogLine to encode null when metrics/tokens are unavailable"
  - "Non-blocking Enqueue with queue-full drop ensures metrics logging never blocks request path"
  - "Daily rotation based on UTC date in filename logs/metrics-YYYY-MM-DD.jsonl"
  - "MetricsPlugin registered in internal/usage init alongside LoggerPlugin"

patterns-established:
  - "Fail-silent logging: write/flush errors are ignored, queue-full drops line"
  - "Plugin composition: multiple usage plugins can coexist (LoggerPlugin + MetricsPlugin)"

# Metrics
duration: 12m
completed: 2026-01-29
---

# Phase 02 Plan 03: Structured Metrics Logging Summary

**按日轮转的 JSONL 指标日志 writer + MetricsPlugin，把每个请求的 TPS/TTFT/TPOT 以结构化格式异步落盘，写失败静默丢弃不影响主流程。**

## Performance

- **Duration:** 12 min
- **Started:** 2026-01-29T16:29:55Z
- **Completed:** 2026-01-29T16:41:00Z
- **Tasks:** 2
- **Files created:** 4

## Accomplishments

- 专用指标落盘包 `internal/metricslog`：定义 `MetricsLogLine` 结构，提供异步单 goroutine JSONL writer，支持按日自动切换文件
- `MetricsPlugin` 实现 `sdk/cliproxy/usage.Plugin`：从 usage record 和 `RequestState` 计算 TPS/TTFT/TPOT，回填到 state 供展示，并 enqueue 到日志 writer
- 进程启动时自动注册：在 `internal/usage/metrics_plugin.go` 的 init 中注册，避免新增 blank import
- 旁路设计保证主链路不受影响：enqueue 非阻塞（队列满直接丢弃），写入/flush/rotate 失败静默处理

## Task Commits

Each task was committed atomically:

1. **Task 1: 实现按日轮转的异步 JSONL writer** - `d2593a1` (feat)
2. **Task 2: 实现 MetricsPlugin** - `a8b5208` (feat)

## Files Created/Modified

- `internal/metricslog/types.go` - MetricsLogLine 结构定义，使用指针字段表示可空的 metrics/tokens
- `internal/metricslog/jsonl_writer.go` - 单 goroutine JSONL writer，带缓冲 channel、定时 flush、按日轮转
- `internal/metricsruntime/usage_plugin.go` - MetricsPlugin 实现，计算指标并 enqueue 日志行
- `internal/usage/metrics_plugin.go` - init 中注册 MetricsPlugin，与 LoggerPlugin 共存

## Decisions Made

- 日志字段使用指针类型（*float64, *int64）以便在 JSON 编码时自然表现为 null
- 队列大小固定 1024，满时直接丢弃该行，确保调用方永不阻塞
- 文件路径固定为 `logs/metrics-YYYY-MM-DD.jsonl`，目录不存在时自动创建
- 定时 flush 间隔 1 秒，进程退出时 flush 剩余数据

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 结构化指标日志已就绪：每个请求产生一行 JSONL，包含完整的 TPS/TTFT/TPOT/token/duration 字段
- 为 Phase 3/4 的持久化与查询提供了原始数据基础
- 日志文件可直接用于离线分析或导入时序数据库

---
*Phase: 02-metrics-collection*
*Completed: 2026-01-29*
