# Phase 4: Query API - Context

**Gathered:** 2026-01-30
**Status:** Ready for planning

<domain>
## Phase Boundary

提供 REST API 查询历史指标数据（来自 SQLite 持久化），并支持：
- `p50/p95/p99` 百分位统计查询
- 按时间窗口（如 `1h`、`1d`）聚合查询

仅澄清“如何实现查询接口的对外契约与行为”。指标采集/写入/保留策略不在本阶段变更。

</domain>

<decisions>
## Implementation Decisions

### API 端点形态
- 采用“统一查询端点”风格（同一个入口覆盖 percentiles 与 buckets 这两类查询）。
- 端点路径语义锁定为 `.../metrics`（不使用 `/metrics/query` 或 `/metrics/history` 这种后缀）。
- 默认不提供“按时间范围返回单请求 raw 列表”；对外主要提供聚合输出。

### 筛选与分组维度
- API 至少支持按 `provider`、`model`、`streaming` 过滤（标准集）。
- 聚合结果固定按 `provider + model + streaming` 分组返回（不是客户端自选 `group_by`）。
- 成功与失败/异常请求的统计结果需要分开输出（两套聚合口径并存）。
- 需要支持按 `request_id` 精确查询单条请求的指标记录（用于排障定位）。

### 时间窗口语义
- 时间范围输入使用 RFC3339 时间戳（例如 `2026-01-30T00:00:00Z`）。
- buckets 的粒度采用“固定集合”（不是任意秒数）。

### 响应格式与单位
- 延迟类指标（TTFT/TPOT 等）以“毫秒整数”返回。
- 百分位输出固定为 `p50/p95/p99`（不做客户端自定义列表）。
- buckets 时间序列中，空 bucket 也要返回：`count=0`，指标字段为 `null`（便于调用方对齐绘图/分析）。

### Claude's Discretion
以下用户明确表示“你来定”，规划/实现阶段可自行定稿且不需要再问：
- URL 是否带版本前缀、以及是否挂载到 public API 还是 management namespace。
- 统一端点内“查询类型选择”的表达方式：`mode=` 参数 vs 子资源路径。
- percentiles 与 buckets 是否支持“一次请求同时返回两者”。
- 未提供时间范围参数时的默认行为：强制必填（400）或默认最近窗口（例如最近 1h）。
- buckets 的对齐规则与默认时区（例如 UTC 自然时间对齐 vs 从 `from` 滚动）。
- buckets 固定集合的具体取值（例如 `1m/5m/15m/1h/1d` 等）。
- 多值过滤是否支持（例如 `provider=a,b` / `model=x,y`）。
- `request_id` 精确查询的返回结构（是否使用与聚合端点一致的 envelope、错误格式等）。

</decisions>

<specifics>
## Specific Ideas

- 端点命名偏好：`/metrics`。
- 百分位固定：`p50/p95/p99`。
- 延迟单位：整数毫秒（`*_ms`）。
- 时间序列需要返回空 bucket（`count=0` + 指标为 `null`），方便下游做可视化对齐。

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 04-query-api*
*Context gathered: 2026-01-30*
