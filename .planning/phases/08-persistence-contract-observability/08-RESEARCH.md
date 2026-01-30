# Phase 8: Persistence Contract & Observability - Research

**Researched:** 2026-01-31
**Domain:** Go 服务内 SQLite best-effort 指标持久化（异步 writer）与 Management API 可观测性契约
**Confidence:** HIGH

## Summary

本阶段要把“best-effort 持久化”从实现细节升级为可审计的语义契约：运行时允许丢行（queue_full / writer_not_started / insert_failure），但必须在 management auth 边界内可观测（degraded），避免用户体验为“静默缺行”。现状中，drop 与 insert failure 都是 **完全静默** 的：`metricspersist.Enqueue` 在 writer 未启动或队列满时直接 return；writer goroutine 的 `Prepare`/`ExecContext` 错误也被吞掉。

实现切入点非常集中：
- **写入路径**在 `internal/metricspersist/writer.go`，把“发生了 drop/写入失败”转成进程级累计计数与最后一次 drop 的时间/原因。
- **对外呈现**在 `internal/api/handlers/management/metrics.go`，复用现有 `meta` 结构，仅当 degraded 时追加 `meta.persistence`（或等价字段）以满足“默认不增加输出”。

测试上建议分两层锁定：
1) `internal/metricspersist` 包内单测：精准驱动 queue_full / writer_not_started / insert_failure 并断言计数与 reason。
2) `test/metrics_management_test.go` 契约测试：在 degraded 与非 degraded 两种情况下验证 JSON 是否出现/不出现 `meta.persistence`，并校验字段不泄露敏感信息。

**Primary recommendation:** 在 `internal/metricspersist` 内建立单一“持久化健康状态”数据源（进程级原子计数 + last drop 记录 + quiet period 计算），由 `internal/api/handlers/management/metrics.go` 在 degraded 时按固定 enum 输出到 `GET /v0/management/metrics` 的 `meta` 中。

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go `database/sql` | stdlib | SQLite 读写接口 | 项目全局使用该抽象层（读侧 QueryContext / 写侧 Prepare+ExecContext） |
| `modernc.org/sqlite` | (go.mod 决定) | 纯 Go SQLite driver | 已作为唯一 SQLite driver 引入（`internal/metricspersist/db.go`) |
| `github.com/pressly/goose/v3` | (go.mod 决定) | SQLite migrations | 已用于启动 migrations（`internal/metricspersist/migrations.go`） |
| `github.com/gin-gonic/gin` | (go.mod 决定) | Management API HTTP handler | Management 路由与 handler 统一基于 Gin（`internal/api/server.go`） |
| Go `sync/atomic` | stdlib | 进程级 drop 计数/状态 | 与“request path 零影响”兼容（无锁快速路径） |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/sirupsen/logrus` | (go.mod 决定) | 启动阶段 fail-fast 日志 | 启动失败路径已使用 `os.Exit(1)` + logrus 记录（`cmd/server/main.go`） |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| 自研 health 暴露端点 | Prometheus / pprof / 自定义 /healthz | 本阶段锁定只在 `/v0/management/metrics` 承载 health，且禁止新增对外产品能力 |

## Architecture Patterns

### Current Implementation Map (关键文件)

**写入链路（best-effort）**
- `internal/metricsruntime/usage_plugin.go`：`MetricsPlugin.HandleUsage` 计算指标后调用 `metricspersist.Enqueue(record)`（仅当 `logging.GetRequestID(ctx)` 非空）。
- `internal/metricspersist/writer.go`：`Enqueue` 非阻塞入队；`StartWriter` 启动 goroutine `run()`；`run()` `Prepare(insert)` + 循环读队列并 `ExecContext`。

**启动阶段 fail-fast**
- `cmd/server/main.go`：启动 server mode 时 `MkdirAll(logs/)` → `metricspersist.InitDB("logs/metrics.db")` → `metricspersist.Migrate(db)` → `metricspersist.StartWriter(db)`；任一步失败 `os.Exit(1)`。

**Management Query API（读侧）**
- `internal/api/server.go`：注册管理路由 `GET /v0/management/metrics` → `s.mgmt.GetMetrics`（management auth 边界内）。
- `internal/api/handlers/management/metrics.go`：组装响应 JSON（`meta` + data/aggregations）。
- `internal/api/handlers/management/handler.go`：懒加载 `metricsReadDB`，默认 `logs/metrics.db`，通过 `metricspersist.InitDB` 打开（读侧不复用 writer connection）。

### Pattern 1: Best-Effort Non-Blocking Enqueue + Background Writer
**What:** request path 只做非阻塞 enqueue；落库在后台 goroutine 完成。
**When to use:** 性能优先、允许丢行但必须可观测的指标持久化。
**Where:** `internal/metricspersist/writer.go`。

**现状 drop 点（必须被观测）**
- `writer_not_started`：`sqliteWriter.Enqueue` 在 `!w.started.Load()` 时直接 return（`internal/metricspersist/writer.go:76-79`）。
- `queue_full`：`select { case w.queue <- record: default: }` 的 default 分支丢弃（`internal/metricspersist/writer.go:80-85`）。
- `insert_failure`：
  - `db.Prepare(insertSQL)` 失败直接 return（`internal/metricspersist/writer.go:117-121`）
  - `stmt.ExecContext(...)` 忽略返回 error（`internal/metricspersist/writer.go:134-149`）

**隐性 drop 点（本期可记录为“非契约 drop”，但不要对外暴露细节）**
- `MetricRecord` 字段缺失：`insert()` 对 `RequestID/Provider/Model` 为空直接 return（`internal/metricspersist/writer.go:124-127`）。
- `request_id` 缺失：`MetricsPlugin` 在 `logging.GetRequestID(ctx)==""` 时完全不 enqueue（`internal/metricsruntime/usage_plugin.go:199-215`）。该问题更像“采集缺口”，与 Phase 10（request_id robustness）有耦合。

### Pattern 2: Management Response Envelope + Meta Echo
**What:** 统一用 `meta` 回显 mode/filters/time-range，并按 mode 返回不同 data shape。
**Where:** `internal/api/handlers/management/metrics.go`。

**现有响应结构（必须保持默认不变）**
- request_id 分支：
  - JSON 顶层：`{"meta": {...}, "data": {...}}` 或 `{"meta": {...}, "error": "..."}`
  - `meta`：`mode=request_id`, `timezone=UTC`, `requested_from/to=null`, `effective_from/to=默认 1h 窗口`, `filters={provider,model,streaming}`
  - `data`（`metricsRow`）：包含 `request_id/provider/model/streaming/status_code/error_info/created_at` 等。
- percentiles 分支：
  - JSON 顶层：`{"meta": {...}, "success": [...], "failure": [...]}`（见 `metricsPercentilesResponse`）
- buckets 分支：
  - JSON 顶层：`{"meta": {...}, "success": [...], "failure": [...]}`（见 `metricsBucketsResponse`）

**最佳插入点（Phase 08）**
- 在 `metricsMeta` 增加 `Persistence *<struct> \`json:"persistence,omitempty"\``（或等价结构），
  - 仅当 `persistence.degraded==true` 才填充指针，默认保持不输出该字段。
  - `degraded` 最小字段：`degraded + dropped_total + last_drop_at`，可选 `last_drop_reason`（enum）。
  - 严格禁止包含 `request_id` 列表、SQL 错误原文、用户输入。

### Anti-Patterns to Avoid
- **在 request path 打日志/打点导致延迟抖动：** drop 统计必须是无锁/低成本；不要在 enqueue 失败时打印 SQL 原文或堆栈。
- **把 best-effort 变成“强保证”：** 不能在 enqueue 处阻塞等待队列空间，也不要在写入失败时重试非幂等写（即便 INSERT+UNIQUE 在多数情况下幂等，也不要引入复杂重试策略）。
- **把 degraded 暴露到普通代理接口：** 本阶段锁定只在 management auth 边界内暴露（`/v0/management/metrics`）。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SQLite driver | 自己嵌入/包装 sqlite C 绑定 | `modernc.org/sqlite` | 项目已统一使用该 driver，避免分裂与 ABI 风险 |
| migrations runner | 自己管理 schema version | `goose/v3` embedded migrations | 已落地且有测试覆盖（`internal/metricspersist/migrations.go`） |
| 并发计数器 | 加 mutex 锁住热路径 | `sync/atomic` | enqueue 是热路径，必须保持非阻塞与低开销 |

**Key insight:** “不丢行”不是目标，“不静默丢行”才是目标；因此要把 drop 作为一等事件（计数 + 最近一次时间 + 原因码），而不是靠日志追查。

## Common Pitfalls

### Pitfall 1: Writer 启动成功但实际不可写（Prepare 失败被吞）
**What goes wrong:** `StartWriter` 返回 nil，但 writer goroutine 在 `db.Prepare` 失败后直接 return，导致持续 enqueue→queue_full→静默 drop。
**Why it happens:** `run()` 对 `Prepare` 错误走 fail-silent；`Start()` 不做任何预检。
**How to avoid:** 在 `StartWriter` 内做一次对 `INSERT INTO metrics ...` 的 `Prepare` 预检（或等价的 schema 就绪检查），失败则让 `StartWriter` 返回 error 触发启动 fail-fast。
**Warning signs:** management 查询侧出现“历史数据突然为空”但服务仍存活（尤其 DB 文件被替换/损坏时）。

### Pitfall 2: 默认响应结构被“悄悄”改变
**What goes wrong:** 给 `/v0/management/metrics` 增加新字段后，客户端解析或测试假设被破坏。
**How to avoid:** 仅在 degraded 时输出 `meta.persistence`；并通过测试显式断言“非 degraded 时 JSON 不包含 persistence 字段”。

### Pitfall 3: 全局 `defaultWriter` + `startOnce` 影响测试可控性
**What goes wrong:** `internal/metricspersist` 的 writer 是进程级单例；一旦 `StartWriter` 被调用，就很难在同一进程内重启或隔离状态。
**How to avoid:** 为测试提供显式 reset（仅测试可用）或将 health tracker 与 writer 实例解耦，使测试能独立设置/清空计数器。

### Pitfall 4: 安全边界泄露
**What goes wrong:** 把 `error.Error()`（可能包含路径/SQL/驱动细节）直接返回给 management 响应，导致信息泄露与不可控的契约漂移。
**How to avoid:** 对外只暴露固定 enum（`queue_full` / `writer_not_started` / `insert_failure`），错误原文仅用于内部日志（且不作为主观测载体）。

## Code Examples

### Example 1: 在 writer hot path 记录 drop（无锁）
```go
// Source: internal/metricspersist/writer.go (to be added)

type DropReason string

const (
    DropReasonQueueFull       DropReason = "queue_full"
    DropReasonWriterNotStarted DropReason = "writer_not_started"
    DropReasonInsertFailure   DropReason = "insert_failure"
)

type PersistenceHealth struct {
    DroppedTotal   uint64
    LastDropAtUTC  *time.Time
    LastDropReason *DropReason
    Degraded       bool
}

// RecordDrop updates counters and last-drop markers.
// Must not allocate or lock on the hot path.
func (w *sqliteWriter) recordDrop(reason DropReason, now time.Time) {
    // atomic.AddUint64(&w.droppedTotal, 1)
    // atomic.StoreInt64(&w.lastDropUnixNano, now.UnixNano())
    // atomic.StoreInt32(&w.lastDropReason, int32(reasonEnum))
}
```

### Example 2: Management API 仅在 degraded 时追加 meta.persistence
```go
// Source: internal/api/handlers/management/metrics.go (to be added)

type metricsPersistenceMeta struct {
    Degraded       bool    `json:"degraded"`
    DroppedTotal   uint64  `json:"dropped_total"`
    LastDropAt     string  `json:"last_drop_at"`
    LastDropReason *string `json:"last_drop_reason,omitempty"`
}

type metricsMeta struct {
    // existing fields...
    Persistence *metricsPersistenceMeta `json:"persistence,omitempty"`
}

health := metricspersist.GetPersistenceHealth(h.nowUTC())
if health.Degraded {
    meta.Persistence = &metricsPersistenceMeta{ /* fill fixed fields */ }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| 静默 best-effort（drop/insert failure 全吞） | best-effort + degraded 可观测（management meta） | Phase 08 (planned) | 运维可发现/可告警，避免“查不到行但不知道为什么” |

**Deprecated/outdated:**
- 依赖 README 示例的 `?limit=10` 查询：当前 handler 不支持 `limit` 且 `mode` 为空会返回 400（`internal/api/handlers/management/metrics.go`）。Phase 08 的契约文档更新应同步修正示例为可执行请求。

## Open Questions

1. **quiet period 的默认值与计算口径**
   - What we know: 允许在静默期后自动恢复为正常（`08-CONTEXT.md:35`）。
   - What's unclear: 具体时长（例如 1m/5m/15m）以及是否基于 last_drop_at 与 nowUTC 计算。
   - Recommendation: 使用 `Handler.nowUTC()` 作为时间基准，quiet period 选一个保守值（例如 5 分钟）并写入契约文本。

2. **`last_drop_reason` 是否默认输出**
   - What we know: degraded 最小字段不包含 reason，但允许作为可选字段（`08-CONTEXT.md:32-47`）。
   - Recommendation: 先实现为可选字段（仅在存在时输出），并在契约文本中声明其为“诊断细节，可能为空”。

3. **嵌入式 SDK 启动路径是否也必须 fail-fast**
   - What we know: 当前 fail-fast 初始化仅在 CLI server mode（`cmd/server/main.go`）执行；嵌入式 `cliproxy.Service` 启动并不显式初始化 metrics DB/writer。
   - Recommendation: planner 需要确认 Phase 08 的“启动 fail-fast”范围是仅 CLI 二进制，还是也覆盖 SDK embedding；若要覆盖 embedding，需要在 `cliproxy` 构建/启动路径补齐同样的初始化。

## Sources

### Primary (HIGH confidence)
- `internal/metricspersist/writer.go` - enqueue/drop 行为、prepare/exec 吞错
- `cmd/server/main.go` - 启动阶段 InitDB/Migrate/StartWriter fail-fast
- `internal/api/handlers/management/metrics.go` - `/v0/management/metrics` 响应结构与 meta 组装
- `internal/api/handlers/management/handler.go` - 读侧 DB lazy open 与默认路径
- `internal/api/server.go` - management 路由注册与 auth 边界
- `test/metrics_management_test.go` - 当前 management metrics 契约测试（meta/time-range/filters）

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - 直接从代码 import 与现有 wiring 得到
- Architecture: HIGH - 直接从关键文件实现与测试得到
- Pitfalls: HIGH - 由现状“吞错/静默 drop”与全局单例模式推导，且有明确代码位置

**Research date:** 2026-01-31
**Valid until:** 2026-03-02
