# Phase 03: Persistence - Research

**Researched:** 2026-01-30
**Domain:** Go (Gin) 指标持久化到 SQLite（`logs/metrics.db`），含 schema / migration / retention / 停用 JSONL
**Confidence:** HIGH

## Summary

本阶段的本质是把 Phase 2 的“每个请求结束时生成一条 MetricsLogLine 并写 JSONL”改造成“每个请求结束时写入 SQLite 一行”，并保证：启动时自动建库/迁移（失败即退出）、写入不影响请求性能、保留最近 7 天数据、DB 成为指标的唯一事实来源。

代码库当前的指标生成点非常清晰：usage pipeline 在请求结束时发布 `usage.Record`（`/home/adam/projects/CLIProxyAPI/internal/runtime/executor/usage_helpers.go`），MetricsPlugin 在 `HandleUsage` 里聚合 TTFT/TPS/TPOT 并落盘（目前落到 JSONL：`/home/adam/projects/CLIProxyAPI/internal/metricsruntime/usage_plugin.go` → `/home/adam/projects/CLIProxyAPI/internal/metricslog/jsonl_writer.go`）。因此 Phase 3 的主要工作是：
- 把落盘介质从 JSONL writer 替换为 SQLite writer（保持非阻塞/旁路特性），
- 引入可失败的 schema 迁移机制并放到“可返回 error 的启动路径”里（而不是 `init()`），
- 统一去重键为 `request_id`（来自现有请求日志中间件），并确保不持久化敏感字段（不存 request path）。

**Primary recommendation:** 使用 `modernc.org/sqlite` 作为 `database/sql` driver（driverName=`sqlite`）+ `pressly/goose` 作为嵌入式迁移工具，启动时 `Up` 迁移 + 7 天清理，运行时由单 writer goroutine 批量 `INSERT ... ON CONFLICT(request_id) DO NOTHING`。

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `database/sql` (Go stdlib) | Go 1.24+ | DB 抽象与连接池 | 项目已是 Go 主栈；与 SQLite driver / migration 工具天然兼容 |
| `modernc.org/sqlite` | 以 go.mod +文档为准 | SQLite driver（纯 Go） | 文档明确 driverName 为 `sqlite`，无需 CGO，利于跨平台构建 |
| `pressly/goose/v3` | 最新稳定版 | SQL migrations（支持 `embed.FS`） | 官方文档提供“嵌入 SQL migrations”的标准做法，适合“启动自动迁移” |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Go `embed` / `io/fs` | Go 1.16+ | 内嵌 migrations 文件 | 迁移 SQL 跟随二进制发布，不依赖外部文件 |
| SQLite PRAGMA（WAL / foreign_keys / busy_timeout 等） | SQLite | 写入稳定性与性能 | 需要 DB 在写入高峰时不因锁冲突频繁失败（即便单实例也会有并发请求写入） |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `pressly/goose` | `golang-migrate/migrate` | migrate 也支持 `embed.FS`（`iofs` source）与 SQLite，但 CLI/driver 组合在社区中更常见坑是“unknown driver sqlite”；goose 的 embed 文档更直接（适合项目内嵌迁移器） |
| 手写迁移（`PRAGMA user_version`） | 自建 migration runner | 容易遗漏幂等、并发、失败恢复与回滚策略；与“fail-fast 自动迁移”目标不匹配 |

## Architecture Patterns

### Recommended Project Structure

建议新增一个“指标持久化”边界包，承接：DB 打开/迁移/写入队列/清理。

```
internal/metricspersist/
├── db.go                # Open + PRAGMA + pool settings
├── migrations.go        # goose provider / embedded FS wiring
├── writer.go            # async writer goroutine + batch insert
└── migrations/
    └── 0001_metrics.sql # initial schema
```

（具体文件名由 planner 决定；重点是“writer / migrations / schema”职责拆开。）

### Pattern 1: Startup Fail-Fast Migration（可失败的启动阶段）

**What:** 在服务真正开始对外提供 HTTP 之前，执行：
1) `logs/` 目录创建 + 打开 `logs/metrics.db`
2) 执行 migrations（Up）
3) 执行 retention cleanup（删除 7 天前数据）

**Where (in this repo):** 需要落在可返回 error 的启动路径，而不是 `init()`：
- `cliproxy.Builder` 支持 `WithHooks`（`/home/adam/projects/CLIProxyAPI/sdk/cliproxy/builder.go`），`Service.Run` 会在 server.Start 前调用 `hooks.OnBeforeStart`（`/home/adam/projects/CLIProxyAPI/sdk/cliproxy/service.go`）。

**Why:** 决策要求“迁移失败则 fail-fast 并退出”，只有启动路径能自然返回 error/中止；`init()` 无法优雅传递错误。

### Pattern 2: Single Writer Goroutine + Batch Inserts（非阻塞写入）

**What:** 复用 Phase 2 JSONL writer 的并发模型：
- 指标生成端（MetricsPlugin）只负责把一条“request-level record” enqueue 到 channel（O(1)，不阻塞）。
- writer goroutine 独占 DB 写入（prepared statement + transaction），按 (N 条或 T 时间) 批量落库。

**Where:** 把 `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/usage_plugin.go` 里现有的 `metricslog.Enqueue(line)` 替换为 `metricspersist.Enqueue(row)`。

**Why:** 决策要求“写入不影响请求性能”。当前 usage pipeline 已是异步分发（`/home/adam/projects/CLIProxyAPI/sdk/cliproxy/usage/manager.go`），但 plugin 的 DB 操作仍可能因锁/IO 阻塞；单 writer + buffer 是最稳定的旁路形态。

### Pattern 3: De-dup by `request_id`（幂等写入）

**What:** 以 `request_id` 作为唯一键：
- 表上 `UNIQUE(request_id)`
- 写入用 `INSERT ... ON CONFLICT(request_id) DO NOTHING`（或等价语义）

**Where (request_id 来源):**
- 请求日志中间件只对 AI API 路径生成 request id（`/home/adam/projects/CLIProxyAPI/internal/logging/gin_logger.go`），并写入 context（`logging.WithRequestID` / `logging.GetRequestID` in `/home/adam/projects/CLIProxyAPI/internal/logging/requestid.go`）。
- `BaseAPIHandler.GetContextWithCancel` 会把 gin ctx 放进 `context.Context`（`/home/adam/projects/CLIProxyAPI/sdk/api/handlers/handlers.go`），usage 发布沿用该 ctx（`/home/adam/projects/CLIProxyAPI/internal/runtime/executor/usage_helpers.go`），因此 MetricsPlugin 可以从 ctx 提取 `request_id`。

**Important mismatch to resolve in planning:**
- 现有 MetricsLogLine 使用 `TrackingID`（UUID）而非 `request_id`；但 Phase 3 决策锁定 dedup key 为 `request_id`。

### Anti-Patterns to Avoid

- **在 `init()` 里做 DB 迁移/打开连接：** 无法优雅返回错误；与“迁移失败就退出”目标冲突。
- **在 MetricsPlugin 里直接 `db.Exec` 写入：** 会让 plugin 受 DB 锁/IO 影响；最坏情况下拖慢 usage 分发线程。
- **写入时仍持久化 `request_path`：** 决策明确禁止；而当前 Phase 2 JSONL 仍包含 `request_path` 字段（必须在 Phase 3 移除/不写入 DB）。

## Don’t Hand-Roll

| Problem | Don’t Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SQL 迁移系统 | 自己维护 `schema_version` 与 SQL 执行顺序 | `pressly/goose`（embed migrations） | 迁移边界、回滚语义、重复执行幂等性等细节很容易踩坑 |
| SQLite 并发写入策略 | 到处 `db.Exec` + 祈祷不锁 | 单 writer goroutine + batch txn | SQLite 锁冲突与 busy 行为复杂；单 writer 模型最可控 |
| “每个连接一次性 PRAGMA” | 启动时 `db.Exec(PRAGMA ...)` 以为全局生效 | DSN/连接级配置 + 控制连接数 | `database/sql` 可能使用多个底层连接；单次 PRAGMA 只对当前连接生效（必须用连接/DSN策略保证一致） |

**Key insight:** 这个 repo 已经有“旁路异步落盘”的成熟模型（JSONL writer）。Phase 3 不应改成“同步写 DB”，而应沿用并升级为“DB writer”。

## Common Pitfalls

### Pitfall 1: `request_id` 与 `tracking_id` 混用导致去重失效

**What goes wrong:** DB schema 以 `request_id` unique，但写入端仍用 `TrackingID`（UUID）作为 key；同一请求重试/重复发布会产生多行。

**How to avoid:** 在 MetricsPlugin 里从 context 读取 `request_id` 并作为唯一键；`TrackingID`（若仍保留）只能作为“内部关联字段”而非 dedup。

**Warning signs:** DB 中同一用户请求出现多行且仅 UUID 不同。

### Pitfall 2: 迁移放错位置导致“失败但继续跑”

**What goes wrong:** 迁移在 goroutine 或 init 中运行，失败只打日志；服务继续接受请求，最终导致“写 DB 的路径静默失败”。

**How to avoid:** 把迁移放在服务启动的同步路径里（server.Start 之前）；返回 error 终止启动。

### Pitfall 3: SQLite 锁冲突导致写入队列堆积/阻塞

**What goes wrong:** 高并发下多 goroutine 同时写 SQLite，出现 SQLITE_BUSY；若写入端阻塞，会拖慢 usage pipeline。

**How to avoid:**
- 单 writer goroutine 串行写入（根治多写者）；
- 需要时设置 busy timeout / txlock（让短暂锁竞争可等待）。注意 busy timeout 并不能保证永不返回 SQLITE_BUSY（SQLite 文档说明存在避免死锁的场景会直接返回）。

### Pitfall 4: Retention delete 没有索引导致启动变慢

**What goes wrong:** 每次启动跑 `DELETE WHERE timestamp < cutoff`，但 timestamp 无索引；数据量上来后启动时间显著增长。

**How to avoid:** 为时间列建索引（或复合索引包含时间）；清理在可控频率下执行（启动一次 + 之后按日/按小时）。

### Pitfall 5: 仍然写 JSONL，导致“双源数据”

**What goes wrong:** DB 写入上线后 JSONL 仍在写，出现两个口径；Phase 4 查询/调试时无法判定“真相来源”。

**How to avoid:** Phase 3 完成后直接停用 JSONL writer（删除调用链或删除包），DB 成为唯一来源。

## Code Examples

### Example 1: 从 ctx 提取 `request_id`

Source: `/home/adam/projects/CLIProxyAPI/internal/logging/requestid.go`

```go
requestID := logging.GetRequestID(ctx)
```

在 MetricsPlugin 的 `HandleUsage(ctx, record)` 内可直接读取；该 ctx 源自 handler 的 `GetContextWithCancel`（已把 request id 传播到 parent ctx）。

### Example 2: modernc SQLite driverName

Source: `modernc.org/sqlite` 文档（pkg.go.dev）

```go
import (
    "database/sql"
    _ "modernc.org/sqlite"
)

db, err := sql.Open("sqlite", "logs/metrics.db")
```

### Example 3: goose 嵌入 migrations（思路）

Source: pressly/goose 官方文档（embed migrations）

```go
//go:embed migrations/*.sql
var migrations embed.FS

// goose.SetBaseFS(migrations)
// goose.Up(db, "migrations")
```

（具体 API 以 planner 实施时的 goose 文档为准；本阶段的关键是“migrations 与二进制一起发布”。）

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| 每条指标写 JSONL 文件（按日轮转） | 每条指标写 SQLite 单表（可索引/可查询） | Phase 3 | Phase 4 的查询能力建立在 DB 之上，避免解析 JSONL |

**Deprecated/outdated:**
- `logs/metrics-YYYY-MM-DD.jsonl`：Phase 3 之后必须停止写入（DB 单一事实源）。

## Open Questions

1. **`logs/metrics.db` 的相对路径基于哪里？**
   - What we know: 决策锁定文件路径字符串为 `logs/metrics.db`。
   - What's unclear: 在生产/容器/Windows 下工作目录是否稳定。
   - Recommendation: planner 明确“以进程工作目录为基准”，并确保启动时创建 `logs/`。

2. **`request_id` 仅对 AI API 路径生成，是否允许 DB 行缺失 request_id？**
   - What we know: request id 中间件只对 `aiAPIPrefixes` 生效（`/home/adam/projects/CLIProxyAPI/internal/logging/gin_logger.go`）。
   - Recommendation: Phase 3 只持久化指标请求（就是 AI API 请求），因此应保证 request_id 始终存在；若未来扩展到其他路径，需先扩展 request id 生成策略。

## Sources

### Primary (HIGH confidence)
- [modernc.org/sqlite (pkg.go.dev)](https://pkg.go.dev/modernc.org/sqlite) - driverName=`sqlite`、DSN/pragma 支持、依赖注意事项
- [Go database/sql connection management (go.dev)](https://go.dev/doc/database/manage-connections) - 连接池与连接级配置的影响
- [pressly/goose embed migrations blog](https://pressly.github.io/goose/blog/2021/embed-sql-migrations/) - 使用 `embed.FS` 内嵌 SQL migrations 的标准方式
- [SQLite WAL documentation](https://sqlite.org/wal.html) - WAL 行为与 checkpoint 基础
- [SQLite busy timeout](https://www.sqlite.org/c3ref/busy_timeout.html) - busy timeout 语义与局限

### Secondary (MEDIUM confidence)
- [golang-migrate sqlite database driver docs](https://pkg.go.dev/github.com/golang-migrate/migrate/v4/database/sqlite) - SQLite driver 选项（如 `x-no-tx-wrap`）与纯 Go driver 说明
- [mattn/go-sqlite3 DSN wiki (for reference)](https://github-wiki-see.page/m/mattn/go-sqlite3/wiki/DSN) - SQLite pragma/DSN 配置的常见项（注意本项目主推 modernc，不是 mattn）

### Tertiary (LOW confidence)
- [golang-migrate issue: unknown driver sqlite](https://github.com/golang-migrate/migrate/issues/899) - CLI/driver 组合的常见踩坑（用于风险提示）

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - modernc/sqlite 与 goose/migrate 都有官方文档支持；项目内需求与约束明确
- Architecture: HIGH - 插入点与数据流已由代码库验证（usage plugin + request id context）
- Pitfalls: HIGH - 主要风险来自明确的当前实现差异（tracking_id vs request_id、request_path）与 SQLite 行为文档

**Research date:** 2026-01-30
**Valid until:** 2026-03-01
