---
phase: quick
plan: 006-10-20-60m-tps-e2e-avg
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/api/handlers/management/metrics.go
  - test/metrics_management_test.go
autonomous: true

must_haves:
  truths:
    - "调用 /v0/management/metrics?mode=buckets 时，每个 bucket 的 metrics 返回 tps_e2e_avg 与 tps_e2e_sample_count"
    - "tps_e2e_avg 仅基于可用样本计算：output_tokens 非空且 duration_ms > 0；并与 buckets 现有口径一致（排除 canceled）"
  artifacts:
    - path: "internal/api/handlers/management/metrics.go"
      provides: "buckets 查询输出新增 tps_e2e_avg/tps_e2e_sample_count"
    - path: "test/metrics_management_test.go"
      provides: "buckets mode 覆盖新字段的 JSON 契约测试"
  key_links:
    - from: "internal/api/handlers/management/metrics.go"
      to: "SQLite metrics 表"
      via: "queryMetricsBuckets SQL 聚合"
      pattern: "tps_e2e"
    - from: "test/metrics_management_test.go"
      to: "/v0/management/metrics?mode=buckets"
      via: "HTTP handler test"
      pattern: "mode=buckets"
---

<objective>
在管理端 buckets 查询中增加端到端吞吐平均值（tps_e2e_avg）及其样本数（tps_e2e_sample_count），用于 10/20/60 分钟窗口汇总时能直接读到“端到端吞吐”指标。

Purpose: 让用户在 buckets 输出中同时看到“端到端吞吐平均值”和“可用样本数”，避免被 NULL 样本误导。
Output: 更新 buckets API JSON 字段 + 对应测试覆盖。
</objective>

<execution_context>
@/home/adam/.config/opencode/get-shit-done/workflows/execute-plan.md
@/home/adam/.config/opencode/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@internal/api/handlers/management/metrics.go
@test/metrics_management_test.go

契约确认（用户已指定）：
- 仅在 `/v0/management/metrics?mode=buckets` 每个 bucket 的 `metrics` 输出新增：
  - `tps_e2e_avg = output_tokens / (duration_ms/1000)`
  - `tps_e2e_sample_count`
- 端到端耗时使用 `duration_ms`（含 TTFT），并与 buckets 当前统计口径一致（排除 canceled）
</context>

<tasks>

<task type="auto">
  <name>Task 1: buckets SQL 聚合新增 tps_e2e_avg + sample_count</name>
  <files>internal/api/handlers/management/metrics.go</files>
  <action>
在 buckets 模式的输出结构与 SQL 查询中新增端到端吞吐字段，且保持现有 buckets 语义不变。

实现要点：
- 在 `metricsBucketsMetrics` 里新增字段：
  - `TPSE2EAvg *float64 \`json:"tps_e2e_avg"\``
  - `TPSE2ESampleCount int \`json:"tps_e2e_sample_count"\``
- 在 `queryMetricsBuckets` 的主查询中追加两列：
  - `SUM(CASE WHEN output_tokens IS NOT NULL AND duration_ms IS NOT NULL AND duration_ms > 0 THEN 1 ELSE 0 END) AS tps_e2e_sample_count`
  - `AVG(CASE WHEN output_tokens IS NOT NULL AND duration_ms IS NOT NULL AND duration_ms > 0 THEN (CAST(output_tokens AS REAL) / (CAST(duration_ms AS REAL) / 1000.0)) END) AS tps_e2e_avg`
- canceled 排除：继续沿用现有 buckets 的过滤/分流逻辑（`status_code=499` 不进入 success/failure 平均值与 count）。
- NULL 规则：当 bucket 内无可用样本时：
  - `tps_e2e_sample_count = 0`
  - `tps_e2e_avg = null`（JSON 里为 null / omitted-by-pointer behavior）
- 在 rows.Scan / 聚合映射处，确保扫描顺序与 SQL 列顺序严格一致，并把 `sql.NullFloat64` 映射为 `*float64` 指针（与现有 `tps_avg` 风格一致）。
- 避免除零：duration_ms <= 0 的行不计入 sample_count，也不参与 AVG。
  </action>
  <verify>
`go test ./...`
  </verify>
  <done>
`/v0/management/metrics?mode=buckets` 的每个 bucket.metrics 包含 `tps_e2e_avg` 与 `tps_e2e_sample_count`，且不改变现有字段行为。
  </done>
</task>

<task type="auto">
  <name>Task 2: 扩展 buckets mode 测试覆盖新字段契约</name>
  <files>test/metrics_management_test.go</files>
  <action>
更新 buckets mode 的测试数据与断言，覆盖新增 JSON 字段：

- 扩展 `bucketsResponse` 结构体（或对应 map 解析）以包含：
  - `tps_e2e_avg` (float64 指针)
  - `tps_e2e_sample_count` (int)
- 在 `seedBucketsMetricsDB` 写入的测试行中补充 `output_tokens` 与 `duration_ms`，保证至少有 1 个 bucket 存在可计算的 tps_e2e：
  - success 行：设置 `output_tokens` 为固定值、`duration_ms` 为已存在的 1234（或其它正数）
  - failure 行：可保持 `output_tokens` 为 NULL（用于验证 sample_count=0 时 avg 为 NULL）
- 在 `TestManagementMetrics_BucketsMode_AlignmentAndEmptyBuckets` 中新增断言：
  - 空 bucket：`tps_e2e_avg` 为 null，`tps_e2e_sample_count` 为 0
  - 有数据的 success bucket：`tps_e2e_sample_count` 为 1 且 `tps_e2e_avg` 与 `output_tokens/(duration_ms/1000)` 近似相等（用已有的 `assertFloatApprox`）
  - 有 failure bucket：若无可用样本则 sample_count=0 且 avg 为 null
- 不新增任何新依赖；沿用现有测试辅助函数。
  </action>
  <verify>
`go test ./...`
  </verify>
  <done>
测试覆盖新增字段的 JSON shape 与口径（含 empty bucket、成功 bucket、失败 bucket），且全套测试通过。
  </done>
</task>

</tasks>

<verification>
- `go test ./...` 全绿
- 手工 spot check（可选）：本地跑一次 `/v0/management/metrics?mode=buckets&bucket=5m&from=...&to=...`，确认响应 JSON 新字段出现且数值合理
</verification>

<success_criteria>
- buckets API 输出新增 `tps_e2e_avg`/`tps_e2e_sample_count`
- canceled 样本仍不污染 buckets 的平均值与 count 语义
- 无可用样本时 avg 为 null 且 sample_count 为 0（与现有 sample_count 风格一致）
</success_criteria>

<output>
After completion, create `.planning/quick/006-10-20-60m-tps-e2e-avg/006-SUMMARY.md`
</output>
