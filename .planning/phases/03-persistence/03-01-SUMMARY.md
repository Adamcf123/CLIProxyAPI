---
phase: 03-persistence
plan: 01
subsystem: database
tags: [sqlite, goose, migrations, modernc]

# Dependency graph
requires:
  - phase: 02-metrics-collection
    provides: request-level metrics generation pipeline (TPS/TTFT/TPOT) and logging infrastructure
provides:
  - SQLite DB init at logs/metrics.db with required PRAGMAs
  - Embedded SQL migrations (goose) for metrics schema
  - Fail-fast startup wiring (exit on init/migration errors)
affects: [03-02, 03-03, 04-query-api]

# Tech tracking
tech-stack:
  added: [modernc.org/sqlite, github.com/pressly/goose/v3]
  patterns:
    - embedded SQL migrations via go:embed
    - startup-time migration with fail-fast exit
    - SQLite PRAGMA configuration for WAL + busy timeout

key-files:
  created:
    - internal/metricspersist/db.go
    - internal/metricspersist/migrations.go
    - internal/metricspersist/migrations/0001_initial_schema.sql
  modified:
    - cmd/server/main.go
    - go.mod
    - go.sum

key-decisions:
  - "Use modernc.org/sqlite (pure Go) as database/sql driver"
  - "Use pressly/goose with embedded migrations and run them in the server startup path"
  - "Set SQLite PRAGMAs (WAL/NORMAL/busy_timeout) and constrain to 1 underlying connection to keep PRAGMAs consistent"

patterns-established:
  - "metricspersist package owns DB open + schema migration entry points"
  - "Server must exit (os.Exit(1)) if DB init or migrations fail"

# Metrics
duration: 5min
completed: 2026-01-30
---

# Phase 3 Plan 1: Persistence Summary

**SQLite metrics schema + embedded migrations (goose) with fail-fast server startup at logs/metrics.db**

## Performance

- **Duration:** 4m40s
- **Started:** 2026-01-30T06:58:58Z
- **Completed:** 2026-01-30T07:03:38Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- 指标持久化基础设施（`internal/metricspersist`）提供 `InitDB` + `Migrate`，并内嵌 SQL migrations
- 初始 schema（`metrics` 表）覆盖 TPS/TTFT/TPOT、token 统计、duration/status/error、created_at，并以 `request_id` 作为主键去重
- 服务启动路径（`cmd/server/main.go`）在启动 HTTP 之前执行 DB init + migration，失败即退出（fail-fast）

## Task Commits

Each task was committed atomically:

1. **Task 1: Create metricspersist package and migrations** - `fd43306` (feat)
2. **Task 2: Integrate into server startup** - `931fdd8` (feat)

## Files Created/Modified

- `internal/metricspersist/db.go` - SQLite 连接初始化与 PRAGMA 配置（WAL/NORMAL/busy_timeout）
- `internal/metricspersist/migrations.go` - goose + go:embed 迁移入口（`Migrate(db)`）
- `internal/metricspersist/migrations/0001_initial_schema.sql` - `metrics` 表初始 schema（`request_id` 主键）
- `cmd/server/main.go` - 启动前创建 logs/ 并执行 DB init + migrations（失败 `os.Exit(1)`）

## Decisions Made

- 使用 `modernc.org/sqlite`（纯 Go）作为 SQLite driver，避免 CGO 依赖
- 使用 `pressly/goose/v3` + `go:embed` 管理迁移，保证 migrations 随二进制发布
- 启动阶段做迁移并 fail-fast，避免“迁移失败但服务继续跑”的静默降级

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added missing Go module dependencies and resolved module graph updates**
- **Found during:** Task 1 (Create metricspersist package and migrations)
- **Issue:** `modernc.org/sqlite` / `github.com/pressly/goose/v3` 未在 go.mod 中，导致编译失败
- **Fix:** 增加依赖并更新 `go.sum`（Go 模块解析同时升级了少量已存在的直接依赖版本）
- **Files modified:** go.mod, go.sum
- **Verification:** `go test ./internal/metricspersist/...` 通过
- **Committed in:** fd43306 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** 仅为完成计划要求的依赖引入，未引入额外功能范围。

## Issues Encountered

- 使用“DB path 为目录”的场景验证 fail-fast 时，modernc sqlite 的报错信息表现为 "out of memory (14)"（本质为无法打开 DB 文件）。已确认服务以 exit code 1 退出。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- DB schema + migrations + 启动 fail-fast 已就绪，可在 03-02 直接接入异步 writer 落库
- 目前 goose 默认会打印迁移日志；若希望统一到 logrus，可在后续计划中做日志口径统一

---
*Phase: 03-persistence*
*Completed: 2026-01-30*
