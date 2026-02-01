---
status: resolved
trigger: "Investigate issue: metrics-null-aggregates"
created: 2026-02-01T13:33:56+08:00
updated: 2026-02-01T13:40:51+08:00
---

## Current Focus

hypothesis: 已确认根因并完成修复与测试验证。
test: 通过 Go 单元测试模拟 streaming SSE chunk → RequestState 记录 first token/chunk count → usage plugin 计算并写入 tps/ttft/tpot。
expecting: 新产生的 streaming metrics 行（output_tokens>=16）应显著降低 tps/ttft/tpot=NULL；management 聚合的 percentiles/buckets 将不再出现大量“有请求但 sample_count=0 导致 percentile=null”。
next_action: 已完成（见 Resolution）。

## Symptoms

expected: 聚合输出（如 percentiles/buckets）里，关键字段（tps/ttft/tpot/status_code 等）应在有数据时有数值；不应出现大量“有请求但聚合为 null”的情况，或至少应能解释/过滤空桶。
actual: 现有 `logs/metrics.db` 中大量行 `tps/ttft/tpot/status_code` 为 NULL；这会在聚合查询中产生大量 NULL（例如 percentile/bucket 字段）。
errors: 暂无明确报错。
reproduction:
  - 直接查 SQLite：
    - `sqlite3 -header -column "logs/metrics.db" "SELECT COUNT(*) AS total, SUM(CASE WHEN tps IS NULL THEN 1 ELSE 0 END) AS tps_null, SUM(CASE WHEN ttft IS NULL THEN 1 ELSE 0 END) AS ttft_null, SUM(CASE WHEN tpot IS NULL THEN 1 ELSE 0 END) AS tpot_null, SUM(CASE WHEN status_code IS NULL THEN 1 ELSE 0 END) AS status_code_null FROM metrics;"`
    - `sqlite3 -header -column "logs/metrics.db" "SELECT request_id, provider, model, streaming, output_tokens, duration_ms, ttft, tpot, tps, status_code, created_at FROM metrics WHERE output_tokens IS NOT NULL AND output_tokens > 0 AND tps IS NULL ORDER BY created_at DESC LIMIT 5;"`
  - 参考文档：`README.md`/`README_CN.md` 描述 `/v0/management/metrics?mode=percentiles|buckets`。
started: 现在库里已有 ~1500 行数据，tps/tpot NULL 占比非常高。

## Eliminated

## Evidence

- timestamp: 2026-02-01T13:33:56+08:00
  checked: user-provided DB snapshot
  found: total~1541; tps_null~1343, ttft_null~1051, tpot_null~1343, status_code_null~1197; 且 output_tokens>0 AND tps IS NULL ~1334
  implication: 大量记录“看似可计算（有 output_tokens/duration_ms/status_code=200）但仍缺失 tps/tpot”，更像写入/更新链路漏填而非真实无数据。

- timestamp: 2026-02-01T13:36:40+08:00
  checked: sqlite3 local counts
  found: total=1560; tps_null=1357; ttft_null=1061; tpot_null=1357; status_code_null=1197
  implication: 问题在当前环境可稳定观察到，且规模与用户快照一致。

- timestamp: 2026-02-01T13:36:40+08:00
  checked: sample rows where output_tokens>0 but tps IS NULL
  found: 多条 streaming=1 且 status_code=200、duration_ms 有值，但 ttft/tpot/tps 仍为 NULL
  implication: 不是“请求失败/usage 缺失”导致；更像 streaming 侧的时间点/内容 chunk 未被记录，触发了 rate 计算抑制。

- timestamp: 2026-02-01T13:36:40+08:00
  checked: metrics persistence writer implementation
  found: `internal/metricspersist/writer.go` 仅 INSERT 且 `ON CONFLICT(request_id) DO NOTHING`，无 UPDATE/补写路径
  implication: 一旦某次 enqueue 写入 NULL，之后不会被补全；必须保证首次写入时字段齐全或改为 upsert/update。

- timestamp: 2026-02-01T13:36:40+08:00
  checked: call sites for `metricsruntime.MaybeRecordFirstContentToken`
  found: 除 `internal/metricsruntime/request_state.go` 定义与测试外，生产代码无任何调用点
  implication: streaming 响应永远不会设置 FirstContentTokenAt/ContentTokenChunks，导致 streaming 的 TTFT/TPS/TPOT 大面积为 NULL。

## Resolution

root_cause: streaming 响应从未调用 `metricsruntime.MaybeRecordFirstContentToken`，导致 `RequestState.FirstContentTokenAt` 与 `ContentTokenChunks` 永远为零值；`metricsruntime.shouldComputeRates` 因缺少 streaming 证据而抑制 TPS/TPOT 计算，从而在 output_tokens>0 的情况下仍把 tps/ttft/tpot 持久化为 NULL（`internal/metricsruntime/usage_plugin.go`）。
fix: 在响应写入层（`internal/api/middleware/response_writer.go`）为 streaming 响应添加 best-effort 记录：在前几个 chunk 上调用 `metricsruntime.MaybeRecordFirstContentToken` 并尽早将状态码写入 RequestState；同时修复 streaming + nil logger 时的潜在 nil deref。
verification: 新增单测：1) `internal/api/middleware/response_writer_metrics_test.go` 断言 streaming SSE content chunk 会设置 FirstContentTokenAt/ContentTokenChunks；2) `internal/metricsruntime/usage_plugin_streaming_rates_test.go` 断言 streaming 且具备证据时会持久化非 NULL 的 tps/ttft/tpot。已运行 `go test ./...` 全绿。
files_changed: [
  "internal/api/middleware/response_writer.go",
  "internal/api/middleware/request_logging.go",
  "internal/api/middleware/response_writer_metrics_test.go",
  "internal/metricsruntime/usage_plugin_streaming_rates_test.go"
]
- timestamp: 2026-02-01T13:40:51+08:00
  checked: SQLite grouped distribution
  found: streaming=1 占绝大多数且 NULL 高发；streaming=0 仅 6 行且 tps/ttft/tpot/status_code 全不为 NULL
  implication: NULL 聚合问题几乎完全由 streaming 路径导致。

- timestamp: 2026-02-01T13:40:51+08:00
  checked: Go tests
  found: `go test ./...` 通过；新增测试覆盖 streaming chunk 记录 first token 以及 streaming usage 计算并持久化 tps/ttft/tpot
  implication: 修复已被自动化验证，且不依赖外部服务/真实 provider。
