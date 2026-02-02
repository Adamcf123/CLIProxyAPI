---
phase: quick
plan: 007-metrics-summary-include-provider-model-w
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/metricsruntime/display.go
  - internal/metricsruntime/usage_plugin.go
  - internal/metricsruntime/request_state.go
  - internal/metricsruntime/display_test.go
  - internal/metricsruntime/usage_plugin_test.go
autonomous: true

must_haves:
  truths:
    - "每个请求结束仍输出单行 stderr `metrics_summary {json}`，且不破坏既有字段含义与命名"
    - "metrics_summary JSON 新增 `window_stats`：按当前请求的 provider/model/streaming 维度，基于 TPSCollector sliding window 输出窗口内 count + tps/ttft/tpot 的 avg（窗口为空则 avg 为 null）"
    - "metrics_summary JSON 新增 `errors_total`：按当前请求的 provider/model/streaming 维度累计到当前；errors=outcome==failure（排除 canceled=499）"
  artifacts:
    - path: "internal/metricsruntime/display.go"
      provides: "metrics_summary JSON 输出新增 window_stats + errors_total"
    - path: "internal/metricsruntime/request_state.go"
      provides: "RequestState 快照包含 window_stats/errors_total"
    - path: "internal/metricsruntime/display_test.go"
      provides: "metrics_summary 新字段（window_stats/errors_total）与口径回归测试"
    - path: "internal/metricsruntime/usage_plugin_test.go"
      provides: "TPSCollector 窗口聚合与 canceled 排除语义仍成立（为 window_stats 提供保障）"
  key_links:
    - from: "internal/metricsruntime/display.go"
      to: "internal/metrics/collector.go"
      via: "MetricsPlugin.HandleUsage 调用 GetWindowStats 并写入 RequestState"
      pattern: "GetWindowStats"
    - from: "internal/metricsruntime/display.go"
      to: "metrics_summary.errors_total"
      via: "MetricsPlugin.HandleUsage 内按 key 累计（failure && !canceled）"
      pattern: "errors_total"
---

<objective>
在每次请求结束输出的 stderr `metrics_summary {}` JSON 中，新增按 provider/model/streaming 维度的窗口聚合统计（window_stats）以及累计错误数（errors_total），用于“截至当前请求”的运行时证据可直接反映同维度健康度与吞吐趋势。

Purpose: 让用户不依赖 management API，仅通过 grep/stderr 就能看到同 provider/model/streaming 的窗口 TPS/TTFT/TPOT 平均值与累计失败次数，方便快速定位某个模型/供应商近期性能退化与错误放量。
Output: 更新 metrics_summary JSON shape（新增字段但不改旧字段）+ 测试回归覆盖。
</objective>

<execution_context>
@/home/adam/.config/opencode/get-shit-done/workflows/execute-plan.md
@/home/adam/.config/opencode/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@internal/metricsruntime/display.go
@internal/metrics/collector.go
@internal/metricsruntime/usage_plugin.go
@internal/usage/metrics_plugin.go
@internal/metricsruntime/display_test.go
@internal/metricsruntime/usage_plugin_test.go

契约确认（用户已指定）：
- 仅变更运行时日志（metrics_summary），不修改 management API。
- 不引入新依赖。
- window_stats 聚合以现有 TPSCollector sliding window 为准（provider/model/streaming key）。
- errors_total 口径：errors = outcome==failure（排除 canceled=499）。
</context>

<tasks>

<task type="auto">
  <name>Task 1: metrics_summary 增加 window_stats + errors_total（保持旧字段不变）</name>
  <files>
internal/metricsruntime/display.go
internal/metricsruntime/usage_plugin.go
internal/metricsruntime/request_state.go
  </files>
  <action>
在 metrics_summary 的序列化结构中追加两个新字段，并保证旧字段行为不变：

1) `window_stats`（对象）：
- 字段名固定为 `window_stats`（避免破坏既有字段；新增即可）。
- 内容至少包含：
  - `count`（int）：窗口样本数
  - `tps_avg`（*float64 -> JSON number|null）
  - `ttft_avg`（*float64 -> JSON number|null）
  - `tpot_avg`（*float64 -> JSON number|null）
- 维度：使用当前请求的 provider/model/streaming 作为 MetricKey。
- 数据源：严格通过 TPSCollector sliding window 的 `GetWindowStats(key)` 获取窗口 stats（不要手写另一个滑动窗口/聚合）。
- 空窗口/无数据时：
  - `count = 0`
  - `tps_avg/ttft_avg/tpot_avg = null`（用指针 nil 表达；不要输出 0 误导）

2) `errors_total`（int）：
- 字段名固定为 `errors_total`。
- 维度同上（provider/model/streaming）。
- 累计口径：当且仅当本次请求 outcome==failure 时累加 1；canceled(499) 不累加。
  - failure 判定建议使用 RequestStateSnapshot：`snap.IsFailure() && !snap.IsClientCanceled()`
  - 递增动作放在 `MetricsPlugin.HandleUsage` 路径中（每个 usage record 只处理一次），保证“截至当前请求”语义。
- 需要并发安全：在 MetricsPlugin 内维护 map+mutex，key 使用 `metrics.MetricKey`（provider/model/streaming）。

3) window_stats 的注入方式：
- 在 `MetricsPlugin.HandleUsage` 计算/更新 TPSCollector 后，调用 `collector.GetWindowStats(key)` 获取窗口统计，并写入 `RequestState`（新增字段）。
- `PrintSummary` 只读取 `RequestStateSnapshot` 中的 window_stats（不直接访问 collector），避免引入进程级全局 wiring。

实现边界：
- 不新增任何依赖。
- 不修改 management API/handler。
- 不改变 `metrics_summary` 现有字段（tracking_id/provider/model/tps/ttft/tpot/tokens/duration/status/path/usage_note）的语义与 null 规则。
  </action>
  <verify>
`go test ./...`
  </verify>
  <done>
- stderr 输出的 `metrics_summary` JSON 中包含 `window_stats` 与 `errors_total` 字段。
- window_stats 取数来源可在代码层面明确指向 TPSCollector.GetWindowStats。
- errors_total 在 failure 且非 canceled 时递增，在 canceled=499 时不递增。
  </done>
</task>

<task type="auto">
  <name>Task 2: 更新 display_test / usage_plugin_test 覆盖 window_stats + errors_total 契约</name>
  <files>
internal/metricsruntime/display_test.go
internal/metricsruntime/usage_plugin_test.go
  </files>
  <action>
补齐/更新现有测试，锁定 metrics_summary 新字段与口径：

1) `internal/metricsruntime/display_test.go`
- 在解析 metrics_summary JSON 的断言中新增：
  - `window_stats` 字段存在（object）
  - `errors_total` 字段存在（number）
- 增加一个回归用例覆盖“窗口非空 + avg 为数值”的路径：
- 增加一个回归用例覆盖“窗口非空 + avg 为数值”的路径：
   - 构造 state，并手动设置 window_stats（通过新的 RequestState setter/字段），然后调用 `PrintSummary(state)` 断言 `window_stats.count >= 1` 且 `tps_avg/ttft_avg/tpot_avg` 为 number。
- 增加 errors_total 语义用例：
  - success 请求：errors_total 不变
  - failure 请求（例如 status=500 或 LastError 非空）：errors_total +1
  - canceled=499：errors_total 不递增
  - 注意在测试开始时重置计数器（测试在同包内可直接清空 package-level map），保证用例互不污染。

2) `internal/metricsruntime/usage_plugin_test.go`
- 在 `TestMetricsPlugin_HandleUsage_CanceledPersists499AndNilErrorInfo` 中补充断言：
  - 取与该请求一致的 MetricKey（provider/model/streaming）调用 `p.collector.GetWindowStats(key)`，应返回 ok=false 或 stats.Count==0（证明 canceled 不进入 TPSCollector 窗口，从而不会污染 window_stats）。
- 可选（同一测试文件内增加新 case）：构造非 canceled 且满足 rates 条件的记录，调用 `p.HandleUsage(...)` 后断言 `p.collector.GetWindowStats(key)` 返回 ok=true 且 Count>0（为 window_stats 的数据源提供稳定证据）。

约束：不新增测试依赖；沿用现有测试风格（map json 解析、go test 全量跑）。
  </action>
  <verify>
`go test ./...`
  </verify>
  <done>
- display_test 覆盖并锁定 metrics_summary 新字段 JSON shape 与 errors_total 口径。
- usage_plugin_test 明确 canceled 不进入 TPSCollector window（保护 window_stats 语义）。
  </done>
</task>

</tasks>

<verification>
- `go test ./...` 全绿
- 抽样 spot check（可选）：跑一次 streaming 请求，grep stderr 的 `^metrics_summary `，确认 JSON 中出现 `window_stats` 与 `errors_total`
</verification>

<success_criteria>
- metrics_summary 仍为单行 JSON，旧字段不变，新字段仅追加。
- `window_stats` 明确基于 TPSCollector sliding window，按 provider/model/streaming 分组。
- `errors_total` 口径稳定：failure 递增、canceled(499) 不计入。
</success_criteria>

<output>
After completion, create `.planning/quick/007-metrics-summary-include-provider-model-w/007-SUMMARY.md`
</output>
