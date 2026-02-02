---
phase: quick
plan: 008-metrics-summary-align-window-stats-error
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/metricsruntime/usage_plugin.go
  - internal/metricsruntime/display_test.go
  - internal/metricsruntime/usage_plugin_test.go
autonomous: true

must_haves:
  truths:
    - "stderr `metrics_summary {json}` 的 `provider`/`model` 字段与 `window_stats`/`errors_total` 的聚合 key 维度一致（同一个 provider/model/streaming）"
    - "聚合 key 优先使用 RequestStateSnapshot.Provider/Model/Streaming（与 metrics_summary 展示一致），当 Provider/Model 缺失时回退到 usage.Record.Provider/Model"
    - "SQLite 落库的 provider/model 语义不变（仍按 usage.Record.Provider/Model 写入；management API JSON shape 不变）"
  artifacts:
    - path: "internal/metricsruntime/usage_plugin.go"
      provides: "为 window_stats/errors_total 统一计算与 metrics_summary 展示对齐的 MetricKey（state 优先，record 回退）"
    - path: "internal/metricsruntime/usage_plugin_test.go"
      provides: "回归：collector window_stats/errors_total key 与 state 展示对齐；且落库 provider/model 不被影响"
    - path: "internal/metricsruntime/display_test.go"
      provides: "回归：metrics_summary JSON shape 不变，仍包含 window_stats/errors_total"
  key_links:
    - from: "internal/metricsruntime/usage_plugin.go"
      to: "internal/metricsruntime/request_state.go"
      via: "RequestStateSnapshot.Provider/Model/Streaming 生成 MetricKey"
      pattern: "Snapshot\(\)"
    - from: "internal/metricsruntime/usage_plugin.go"
      to: "internal/metrics/collector.go"
      via: "CompleteRequest 与 GetWindowStats 使用同一个聚合 key"
      pattern: "GetWindowStats\(|CompleteRequest\("
---

<objective>
让 stderr `metrics_summary` 中的 `window_stats` / `errors_total` 聚合维度与当前行展示的 `provider`/`model` 对齐，避免出现“显示是 A 模型，但窗口统计/错误累计却按 B 模型分组”的运行时证据错位。

Purpose: 运行时排障时（grep `metrics_summary`）可以直接信任同一行里 provider/model 与窗口统计/错误累计描述的是同一组流量。
Output: 仅调整运行时聚合 key 的选择逻辑 + 测试回归；SQLite/provider/model 落库语义与 management API 不变。
</objective>

<execution_context>
@/home/adam/.config/opencode/get-shit-done/workflows/execute-plan.md
@/home/adam/.config/opencode/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@internal/metricsruntime/usage_plugin.go
@internal/metricsruntime/request_state.go
@internal/metricsruntime/display.go
@internal/metricsruntime/display_test.go
@internal/metricsruntime/usage_plugin_test.go

约束（用户已指定）：
- 聚合 key 优先 RequestState.Provider/Model/Streaming（与 metrics_summary 展示一致），缺失时回退 usage record provider/model。
- 不改变 SQLite 落库 provider/model 语义（management API 不变）。
- 不引入新依赖。
</context>

<tasks>

<task type="auto">
  <name>Task 1: 统一 window_stats/errors_total 的聚合 key（state 优先，record 回退）</name>
  <files>
internal/metricsruntime/usage_plugin.go
  </files>
  <action>
在 MetricsPlugin.HandleUsage 内，把“用于 TPSCollector window_stats + errors_total 的聚合 key”改为与 metrics_summary 展示对齐：

1) 增加一个小的纯函数/私有 helper（放在 `internal/metricsruntime/usage_plugin.go` 内即可，不新增依赖）：
- 输入：RequestStateSnapshot + usage.Record
- 输出：`metrics.MetricKey` + `ok bool`
- 规则：
  - Provider：优先 `snap.Provider`（trim 后非空），否则用 `record.Provider`
  - Model：优先 `snap.Model`（trim 后非空），否则用 `record.Model`
  - Streaming：使用 `snap.Streaming`
  - ok：当 Provider/Model 最终仍为空时返回 false（避免把空字符串作为全局聚合桶）

2) 在计算 TPS/TPOT 的路径里，`RequestMetrics.Key` 使用上述 key（只有 ok==true 才允许进入 `CompleteRequest`），确保：
- 本次请求写入 sliding window 的分组 key 与后续 `GetWindowStats(key)` 查询一致。

3) 注入 metrics_summary 字段时：
- `state.SetWindowStats(...)` 与 `state.SetErrorsTotal(...)` 使用同一个 key（state 优先，record 回退）
- errors_total 的递增口径保持不变：`snap.IsFailure() && !snap.IsClientCanceled()`

明确边界：
- `provider`/`model` 的 SQLite 持久化字段继续使用 `record.Provider`/`record.Model`（不要改为 state 版本）。
- 不修改 `internal/metricsruntime/display.go` 的 `metrics_summary.provider/model` 输出来源（仍然是 state snapshot）。
  </action>
  <verify>
`go test ./...`
  </verify>
  <done>
- window_stats/errors_total 的分组 key 与 metrics_summary 展示的 provider/model 对齐；Provider/Model 缺失时才回退 record。
- SQLite 落库 provider/model 字段仍来自 usage.Record（语义不变）。
  </done>
</task>

<task type="auto">
  <name>Task 2: 补齐回归测试：对齐 key + 不影响落库 provider/model</name>
  <files>
internal/metricsruntime/display_test.go
internal/metricsruntime/usage_plugin_test.go
  </files>
  <action>
更新/新增测试用例，锁定本次“对齐 key”的契约：

1) `internal/metricsruntime/usage_plugin_test.go`
- 在 `TestMetricsPlugin_HandleUsage_PopulatesCollectorWindowStatsForNonCanceled` 增加一个子用例：
  - record.Provider/Model 设置为 A（例如 openai/gpt-5.2）
  - state.SetProvider/SetModel 设置为 B（例如 anthropic/claude-3.5）
  - 调用 `HandleUsage` 后：
    - 以 key=B + streaming=state.Snapshot().Streaming 调 `p.collector.GetWindowStats(key)` 必须 ok=true 且 Count>0
    - 以 key=A 调 `GetWindowStats` 必须 ok=false 或 Count==0（证明聚合 key 确实以 state 为准）

- 在一个捕获落库的测试（已有 `enqueueMetricRecord` stub 的测试）里补充断言：
  - 即使 state.Provider/Model 与 record 不同，`captured.Provider`/`captured.Model` 仍等于 `record.Provider`/`record.Model`
  - 以此锁定“SQLite/provider/model 语义不变”的要求

- 同步更新 `TestMetricsPlugin_HandleUsage_CanceledPersists499AndNilErrorInfo`：
  - 它当前用 record.Provider/Model 构造 key；将其改为“用 state 优先后的 key”（可在测试里直接用 state.SetProvider/SetModel 制造差异，再按新规则构造 key）

2) `internal/metricsruntime/display_test.go`
- 维持现有 shape 断言即可；若有测试依赖了旧的 key 假设（间接），一并修正。

约束：不新增依赖；测试仍用 `go test ./...` 全量通过。
  </action>
  <verify>
`go test ./...`
  </verify>
  <done>
- 测试覆盖并锁定：collector window_stats/errors_total 的聚合 key 与 metrics_summary 展示对齐（state 优先，record 回退）。
- 测试覆盖并锁定：SQLite 落库 provider/model 不受此改动影响。
  </done>
</task>

</tasks>

<verification>
- `go test ./...` 全绿
- spot check（可选）：跑一次请求，确认同一行 `metrics_summary` 的 provider/model 与 window_stats/errors_total 语义一致
</verification>

<success_criteria>
- `metrics_summary.provider/model` 与 `metrics_summary.window_stats/errors_total` 的分组维度一致；不再出现错位。
- SQLite/provider/model 落库语义保持 record 来源不变（management API 不受影响）。
</success_criteria>

<output>
After completion, create `.planning/quick/008-metrics-summary-align-window-stats-error/008-SUMMARY.md`
</output>
