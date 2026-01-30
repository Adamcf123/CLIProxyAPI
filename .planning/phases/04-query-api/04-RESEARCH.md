# Phase 4: Query API - Research

**Researched:** 2026-01-30
**Domain:** Gin REST API + SQLite analytical queries (percentiles + time buckets)
**Confidence:** HIGH

## Summary

本阶段要把“历史指标查询”从 SQLite 暴露成一个稳定的 REST contract：同一个 `/.../metrics` 入口同时支持 percentiles（p50/p95/p99）与 buckets（固定粒度时间序列），并且聚合输出按 `provider + model + streaming` 固定分组，同时把“成功请求”和“失败/异常请求”分开统计。

实现上要优先复用项目现有栈与既有统计语义：HTTP 层使用 Gin（项目已全量使用），鉴权建议挂到现有 management namespace（`/v0/management`）并复用 `managementHandlers.Handler.Middleware()`；统计计算复用 `internal/metrics/window.go` 的“线性插值百分位算法”以保持和 Phase 1 的百分位定义一致；SQL 层只做过滤/粗聚合（按 bucket、按分组、按成功/失败切片），避免在 SQLite 内做复杂 percentile SQL。

需要特别注意两个当前代码库层面的“硬约束/缺口”：
1) `metrics` 表当前没有 `streaming` 列，但 Phase 4 的 locked decision 要求按 `streaming` 过滤且分组返回；没有 schema/migration 变更无法满足（见 `internal/metricspersist/migrations/0001_initial_schema.sql`）。
2) 进程内写入 SQLite 使用一个独立 writer goroutine；为了避免查询阻塞写入，Query API 不应复用 writer 的 `*sql.DB` 连接池，而应为查询侧单独 `sql.Open()` 同一 DB 文件（WAL 下读写可并发）。

**Primary recommendation:** 将 Query API 实现为 management 受保护端点 `GET /v0/management/metrics`（统一入口），查询侧单独打开 SQLite 连接，并在 Go 侧按既有百分位算法计算 p50/p95/p99 + 生成空 buckets。

## Standard Stack

### Core
| Library | Version (repo) | Purpose | Why Standard |
|---|---:|---|---|
| `github.com/gin-gonic/gin` | v1.10.1 | HTTP 路由与 JSON 响应 | 项目主 API 与 management API 已统一使用 Gin（`internal/api/server.go`） |
| Go `database/sql` | go1.24 标准库 | SQLite 查询执行 | 项目持久化已基于 `database/sql`（`internal/metricspersist/db.go`） |
| `modernc.org/sqlite` | v1.44.3 (indirect) | SQLite driver | Phase 3 已落地，避免引入新 driver |

### Supporting
| Library | Version (repo) | Purpose | When to Use |
|---|---:|---|---|
| `github.com/pressly/goose/v3` | v3.26.0 (indirect) | SQLite migrations | 需要为 Query API 增补 schema（如 `streaming` 列、索引）时继续沿用 |
| `github.com/stretchr/testify` | v1.11.1 | 测试断言 | 管理端点测试当前大量使用 |

**Installation:** 无（本阶段应复用现有依赖，避免新增第三方库）。

## Architecture Patterns

### Existing HTTP Stack / Routing Style (必跟随)

- **Gin engine 构建**：`internal/api/server.go` 使用 `gin.New()` + 自定义 middleware（`GinLogrusLogger`/`GinLogrusRecovery`/CORS/RequestLoggingMiddleware）。
- **鉴权与命名空间**：management 路由挂载于 `GET/PUT/... /v0/management/*`，由 `managementAvailabilityMiddleware()` + `managementHandlers.Handler.Middleware()` 保护（`internal/api/server.go` + `internal/api/handlers/management/handler.go`）。
- **错误格式**：management handlers 约定 `c.JSON(status, gin.H{"error": "..."})`（见 `internal/api/handlers/management/logs.go`、`internal/api/handlers/management/usage.go`）。

**强制结论（建议落点）：** Query API 作为“历史指标查询”属于敏感/内部数据面，默认挂载在 management namespace：
- `GET /v0/management/metrics`（统一入口：percentiles/buckets/request_id）

### Data Access Pattern (SQLite)

现状（Phase 3 已落地）：
- DB 初始化与 migrations 在进程启动时完成（`cmd/server/main.go` 调 `metricspersist.InitDB` + `metricspersist.Migrate`）。
- metrics 写入走 `metricspersist.Enqueue` 的后台 writer（`internal/metricspersist/writer.go`）。

**推荐用于 Query API 的读路径：**
- **为查询侧单独打开一个 `*sql.DB`**（指向同一 `logs/metrics.db` 文件），并应用与写入端一致的 PRAGMA（至少 `journal_mode=WAL`、`synchronous=NORMAL`、`busy_timeout`）。理由：写入端当前连接池是单连接（`SetMaxOpenConns(1)`），如果查询复用同一 `*sql.DB` 会把“读操作”与“写操作”串行化。

### Query API Contract Pattern (统一入口)

根据 locked decisions，“统一查询端点”建议用 query 参数表达查询类型：

- `GET /v0/management/metrics?request_id=...` → 单条记录查询（排障）
- `GET /v0/management/metrics?mode=percentiles&from=...&to=...&provider=...&model=...&streaming=...` → 百分位聚合
- `GET /v0/management/metrics?mode=buckets&from=...&to=...&bucket=...&provider=...&model=...&streaming=...` → 时间序列 buckets

（`mode`/是否允许一次返回两种聚合、默认时间范围等属于 Claude's Discretion；planner 可直接定稿，不必回问。）

### Anti-Patterns to Avoid

- **把 percentiles 下推到 SQLite 复杂 SQL：** SQLite 原生不提供 `percentile_cont`；用窗口函数/排名硬做可读性差且容易和既有线性插值定义不一致。
- **复用 writer 的 `*sql.DB` 连接池做读查询：** 会让“写入队列”被读阻塞（尤其是 buckets 查询）。
- **返回 raw 列表作为默认：** locked decision 明确“默认不提供按范围返回单请求 raw 列表”。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---|---|---|---|
| 百分位算法 | 在 Query API 里重新定义一套 percentile | 复用/抽出 `internal/metrics/window.go` 的线性插值算法 | 保证 percentiles 与 Phase 1（p95/p99 线性插值）语义一致，避免“同名不同义” |
| RFC3339 解析 | 手写时间字符串解析 | `time.Parse(time.RFC3339, ...)` | 输入契约锁定为 RFC3339 |
| SQL 拼接 | 字符串拼接构造 WHERE/IN | `database/sql` 参数化 + 动态 placeholder 生成 | 防注入 + 更稳定 |
| 空 buckets 生成 | 试图用 SQLite 生成序列（复杂递归 CTE） | Go 侧生成 bucket 时间轴 + map 回填 | 更易保证“空 bucket count=0, metrics=null”契约 |

**Key insight:** 这个阶段的复杂点不在“SQL 写法”，而在“契约一致性”（百分位定义、空 bucket 行为、成功/失败分离、单位转换）。把可变复杂度留在 Go 侧更可控。

## Common Pitfalls

### Pitfall 1: `streaming` 维度在 SQLite schema 缺失（阻断性）
**What goes wrong:** API contract 要求按 `streaming` 过滤并以 `provider+model+streaming` 分组，但 `metrics` 表当前没有 `streaming` 列，无法在 DB 层筛选/分组。
**Why it happens:** Phase 3 的 `metrics` schema（`internal/metricspersist/migrations/0001_initial_schema.sql`）只存了 `provider/model/tps/ttft/tpot/...`。
**How to avoid:** 需要新增 migration：
- `ALTER TABLE metrics ADD COLUMN streaming INTEGER;`（推荐 0/1，nullable 或 NOT NULL + default）
- writer 入库时写入 streaming（来自 `RequestStateSnapshot.Streaming`）
**Warning signs:** planner 里如果试图“从现有列推导 streaming”，基本都会失败（当前也没有 request_path）。

### Pitfall 2: 时间比较/时区不一致导致 off-by-one bucket
**What goes wrong:** `created_at` 存储为 `CURRENT_TIMESTAMP`（无时区后缀），客户端输入 RFC3339（带 `Z` 或 offset），如果比较/对齐规则不一致，会出现 bucket 边界错位或遗漏。
**How to avoid:**
- SQL 使用 `datetime(?)` 解析 RFC3339，并在比较中统一到 UTC（SQLite date/time functions 内部使用 UTC，RFC3339 的 `Z`/offset 会被规范化）。
- bucket 对齐规则明确为 UTC（例如按自然 UTC 时间对齐到分钟/小时边界）。
**Source:** SQLite date/time functions 文档说明支持 ISO-8601（含 `T`、`Z`、offset）并使用 UTC 处理（https://www.sqlite.org/lang_datefunc.html）。

### Pitfall 3: NULL 指标值污染 percentiles/avg
**What goes wrong:** TPS/TPOT/TTFT 在某些请求上会是 NULL（比如 token usage 缺失或抑制计算），如果把 NULL 当 0 会显著拉低统计值。
**How to avoid:**
- 计算 percentiles/avg 时仅使用非 NULL 样本；如果样本为空，则输出 `null`。
- `count` 语义要明确：是“请求数”还是“参与该指标计算的样本数”。建议同时返回 `count`（请求数）与 `sample_count`（该指标非 NULL 数）以便解释。

### Pitfall 4: buckets 空洞不返回导致下游绘图困难
**What goes wrong:** 只返回有数据的 bucket，调用方无法对齐时间轴。
**How to avoid:** Go 侧根据 `from/to/bucket` 生成完整 bucket 序列，每个 bucket 都输出；无数据时 `count=0` 且指标字段为 `null`（locked decision）。

### Pitfall 5: 查询阻塞写入（影响实时指标可见性）
**What goes wrong:** buckets 查询可能扫描大量行；如果复用 writer 的 `*sql.DB`（且 `SetMaxOpenConns(1)`），会造成写入 goroutine 阻塞。
**How to avoid:** Query API 使用独立 `*sql.DB` 连接池（同文件），WAL 下读写可并行。

## Code Examples

### SQLite: RFC3339 filter（建议）
```sql
-- Source: https://www.sqlite.org/lang_datefunc.html
SELECT request_id, provider, model, created_at
FROM metrics
WHERE created_at >= datetime(?) AND created_at < datetime(?)
ORDER BY created_at ASC;
```

### Bucket start time（按秒对齐；由 Go 传入 bucket_seconds）
```sql
-- Source: https://www.sqlite.org/lang_datefunc.html
-- bucket_start = datetime(floor(unixepoch(created_at)/bucket_seconds)*bucket_seconds, 'unixepoch')
SELECT
  provider,
  model,
  datetime((unixepoch(created_at) / ?) * ?, 'unixepoch') AS bucket_start,
  COUNT(*) AS count,
  AVG(tps) AS tps_avg
FROM metrics
WHERE created_at >= datetime(?) AND created_at < datetime(?)
GROUP BY provider, model, bucket_start
ORDER BY bucket_start ASC;
```

（注意：上例展示 SQL 可做的“粗聚合”；percentiles 仍建议在 Go 侧算，空 buckets 也在 Go 侧补齐。）

### Percentiles algorithm（线性插值；与项目现有一致）
```go
// Source: internal/metrics/window.go (calculatePercentile)
// index = (p/100) * (n-1)
// interpolate between floor(index) and ceil(index)
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|---|---|---|---|
| JSONL 作为持久化 | SQLite + goose migrations + writer goroutine | Phase 03 | 查询 API 可以依赖 DB 作为单一事实来源 |

**Outdated:**
- 依赖 JSONL 历史文件进行聚合（Phase 03 已 decommission）。

## Testing Recommendations

- **Handler contract tests（Gin + httptest）**：覆盖 query 参数校验（缺失/非法 RFC3339、from>to、非法 bucket、非法 streaming 值、mode 未知）。
- **Deterministic aggregation tests（temp sqlite file）**：在 `t.TempDir()` 里初始化 DB（沿用 `metricspersist.InitDB` + `metricspersist.Migrate`），插入固定数据集后调用 API：
  - percentiles：验证 p50/p95/p99 与线性插值一致（含偶数/奇数样本、边界样本）。
  - buckets：验证空 bucket 必返回、count=0、指标 null；验证 bucket 对齐（UTC）与边界 `[from,to)` 语义。
  - success vs error：插入 `status_code=200` 与 `status_code=500`（或 error_info 非空）分别验证两套聚合输出。
- **Request ID lookup tests**：存在返回 200 + 结构化记录；不存在返回 404（或 200 + null，需在 contract 中锁定）。

## Open Questions

1. **`streaming` 缺失的 schema 变更是否允许归入 Phase 4？**
   - What we know: locked decision 要求按 streaming 过滤/分组；当前表无该列。
   - What's unclear: Phase boundary 描述说“不变更写入/保留策略”，但 schema 补齐是查询能力的前置条件。
   - Recommendation: planner 直接把“新增 streaming 列 + writer 入库填充 +（可选）回填策略”列为 Phase 4 的第一步，并在变更说明里明确这是为满足 STOR-02/03/04 的必要条件。

2. **成功/失败划分规则**
   - What we know: DB 有 `status_code` 与 `error_info`；locked decision 要求两套口径并存。
   - What's unclear: 成功是否仅 `2xx`，还是 `<400`；error_info 是否作为失败强信号。
   - Recommendation: 在 contract 中固定：`success = status_code >=200 AND status_code <300 AND (error_info IS NULL OR error_info = '')`；其余归为 `failure`。

3. **默认时间范围与 bucket 对齐**
   - What we know: 输入时间使用 RFC3339；bucket 粒度固定集合；空 bucket 必返回。
   - What's unclear: 未提供 from/to 时是否默认最近 1h；bucket 是 UTC 自然边界对齐还是从 from rolling。
   - Recommendation: 选择“UTC 自然边界对齐 + 默认最近 1h（to=now, from=now-1h）”，并在响应中回显 `effective_from/effective_to`。

## Sources

### Primary (HIGH confidence)
- Repo code: `internal/api/server.go`（Gin engine + management routes）
- Repo code: `internal/api/handlers/management/handler.go`（management 鉴权与错误格式）
- Repo code: `internal/metricspersist/migrations/0001_initial_schema.sql`（metrics schema，缺 streaming）
- Repo code: `internal/metricspersist/db.go` + `internal/metricspersist/writer.go`（SQLite PRAGMA + writer 并发模型）
- Repo code: `internal/metrics/window.go`（线性插值 percentile 实现）
- SQLite docs: https://www.sqlite.org/lang_datefunc.html（ISO-8601/RFC3339 时间解析与 UTC 语义）

### Secondary (MEDIUM confidence)
- SQLite window functions docs: https://www.sqlite.org/windowfunctions.html（可选的 SQL 端聚合思路；本阶段不建议主路径依赖）

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - 直接来自 `go.mod` 与现有代码路径
- Architecture: HIGH - 现有 Gin/management 路由结构明确
- Pitfalls: HIGH - streaming 列缺失/连接池串行化为代码级事实

**Research date:** 2026-01-30
**Valid until:** 2026-03-01
