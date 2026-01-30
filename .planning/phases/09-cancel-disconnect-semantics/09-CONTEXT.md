# Phase 09: Cancel/Disconnect Semantics - Context

**Gathered:** 2026-01-31
**Status:** Ready for planning

<domain>
## Phase Boundary

本阶段只做一件事：把“客户端取消/断连（cancel/disconnect）”的语义明确下来，并保证写入层/查询层/聚合层一致。

交付结果必须满足：Query API 不会把 canceled 误归类为 success，从而避免污染聚合与用户判断。

不在本阶段范围内：新增产品能力（例如新增新端点/新查询参数以控制语义）、扩大数据面到更多诊断字段、或引入额外的运行时兼容模式。

</domain>

<decisions>
## Implementation Decisions

### Outcome 分类规则（Success / Failure / Canceled）
- 流式请求在未自然结束前发生客户端断连/取消：归类为 `canceled`。
- 非流式请求在服务端完整写出响应前发生客户端断连/取消：归类为 `canceled`。
- timeout（deadline exceeded / upstream timeout / server-side timeout）：归类为 `failure`（不算 canceled）。
- 上游明确返回错误（4xx/5xx 或流式 terminal error）：归类为 `failure`。
- 断连发生在服务端已经自然结束并完整写出响应之后：归类为 `success`。
- 判定倾向：conservative canceled —— 只要服务端观察到下游连接已断/请求被取消，就倾向标记为 canceled（宁可不算 success）。

### 聚合口径（percentiles / buckets）
- `mode=percentiles`：`canceled` 请求不参与 percentiles 计算（避免半截样本污染统计）。
- `mode=buckets`：每个 bucket 需要单独统计 `canceled_count`，不并入 failure。
- 聚合分母/total：`total = success + failure + canceled`（canceled 计入总请求数）。
- tokens/throughput 等聚合：`canceled` 的 usage（即使存在）也不计入 token 聚合。

### 对外契约（Query API 输出方式）
- 对外必须显式区分 canceled：查询结果中存在明确的 `canceled` outcome/status（不是把 canceled 塞进 failure）。
- buckets 输出：新增 `canceled_count` 字段（与 success/failure 并列），而不是单独一套 canceled buckets 结构。
- percentiles 输出：需要在响应中显式体现 canceled 被排除的事实（例如提供 `canceled_count` 的元信息）。
- per-request 查询（`request_id=...`）：当 outcome 为 canceled 时，保持 error 为空（不把 canceled 表现为故障错误）。
- 兼容性策略：允许破坏性变更；若需要，直接在 `/v0/management/metrics` 原地调整（不做双写/兼容窗口），并一次性完成切换（hard cutover）。

### 必须覆盖的边界场景（测试/契约锁定）
- streaming vs non-streaming 两条路径都必须覆盖。
- “已输出部分 token/chunk 后断连/取消”仍应归类为 canceled。
- “上游已完成但客户端未收到完整响应”以客户端体验为准（归类 canceled）。
- “服务端自然结束并完整写出响应后才断连”归类 success。

### Claude's Discretion
- canceled 的具体字段命名（除 `canceled` 拼写已锁定外）与放置位置（顶层/record 内/统计内）由实现与现有响应结构决定。
- 是否需要在 meta 中额外写出“percentiles 排除 canceled”这一规则的文字说明（在不扩大输出噪音的前提下）。

</decisions>

<specifics>
## Specific Ideas

- 核心目标是避免“客户端取消/断连被当作 success”导致的聚合污染；因此选择 conservative canceled + 聚合上单列 canceled。

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 09-cancel-disconnect-semantics*
*Context gathered: 2026-01-31*
