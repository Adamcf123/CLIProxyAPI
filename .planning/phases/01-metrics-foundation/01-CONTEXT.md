# Phase 1: Metrics Foundation - Context

**Gathered:** 2026-01-29
**Status:** Ready for planning

<domain>
## Phase Boundary

建立完整的指标计算能力，支持 TPS、TTFT、TPOT 计算，并按 provider/model/streaming 分组统计。输出是可被下游阶段调用的指标计算模块。

</domain>

<decisions>
## Implementation Decisions

### 指标精度
- TPS 保留 2 位小数（如 23.45 tokens/s）
- TTFT 和 TPOT 使用秒为单位（如 0.15 s）
- 仅统计输出 tokens，不区分输入/输出
- Token 数量来源：使用 Provider 返回的 usage.completion_tokens

### 分组维度
- 基本分组：provider + model
- 额外分组：区分 streaming / non-streaming
- 不区分：API endpoint、用户/调用者

### 聚合策略
- 单次请求指标 + 滑动窗口聚合
- 滑动窗口大小：100 次请求
- 滑动窗口维度：每个 provider + model 组合分别统计
- 滑动窗口统计的指标：TPS、TTFT、TPOT 全部
- 滑动窗口中的统计值：最小值、最大值、平均值、中位数、p95、p99
- 滑动窗口存储方式：内存中循环覆盖，满了后自动替换最老数据
- 滑动窗口持久化：进程重启后需要恢复数据

### 边界情况处理
- 请求超时（部分响应）：丢弃该请求，不记录任何指标
- 空响应（0 token）：丢弃该请求，不记录任何指标
- Provider token 数与 chunk 数不一致：以 Provider 返回的 usage 为准
- 非流式请求的 TTFT：等于总响应时间

### Claude's Discretion
- 滑动窗口的具体实现方式（环形队列、双端队列等）
- 滑动窗口的序列化格式
- 百分位数计算算法（是否需要近似算法优化）
- 内存中窗口数据的清理策略

</decisions>

<specifics>
## Specific Ideas

- 指标计算需要适配不同 provider 的响应格式（OpenAI、Gemini、Claude）
- 每个请求生成唯一的 tracking ID 用于指标关联（来自 ROADMAP.md 要求）

</specifics>

<deferred>
## Deferred Ideas

- 按 API endpoint 分组统计 — 当前 phase 不需要
- 按用户/调用者分组 — 当前 phase 不需要
- 累计统计（启动以来的平均值）— 如需要可后续添加

</deferred>

---

*Phase: 01-metrics-foundation*
*Context gathered: 2026-01-29*
