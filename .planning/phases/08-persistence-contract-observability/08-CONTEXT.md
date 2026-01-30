# Phase 8: Persistence Contract & Observability - Context

**Gathered:** 2026-01-31
**Status:** Ready for planning

<domain>
## Phase Boundary

本阶段交付一个清晰、可执行、可验证的“best-effort 指标持久化”语义契约，并补齐可观测性，避免用户侧体验为“静默缺行”。

范围包含：对外契约文本（哪些场景允许丢行、如何理解）、关键丢弃路径的可观测信号（可追踪且不弱化安全边界）、以及至少一套回归/契约测试覆盖可观测性信号。

范围不包含：把 best-effort 改成强保证（例如“永不丢行”）；也不包含新增与管理查询无关的新产品能力。

</domain>

<decisions>
## Implementation Decisions

### Drop 语义边界（best-effort contract）
- 启动阶段如果 SQLite 持久化不可用（无法打开/迁移 `logs/metrics.db` 或 writer 无法启动）：`fail fast`（拒绝启动）。
- 运行中出现 queue-full / insert failure / writer-not-started 等非致命场景：允许 best-effort 丢行，但必须通过可观测性暴露（禁止“静默缺行”）。
- 丢行在运维语义上属于 `degraded`（服务可用但 metrics 不完整），需要可发现/可告警的信号。
- 对外契约文本需要明确列出允许丢行的场景（例如：`queue_full`、`writer_not_started`、`insert_failure`），便于审计复核与排障。

### 对外呈现方式（面向运维/管理员）
- “持久化是否完整/是否发生丢行”的呈现只面向运维/管理员（不在普通调用方的代理响应中暴露）。
- 主入口在 Management API（以 `/v0/management/metrics` 的响应为入口承载 health 信息）。
- 默认情况下不增加输出；仅在出现 `degraded` 时返回额外的 health/meta 信息（不引入新的查询参数/开关）。
- `degraded` 时 API 输出的最小集合：状态 + 聚合丢弃计数（后续在安全区约束下允许少量诊断细节，见下）。

### 可观测性载体（避免静默缺行）
- 除 Management API 之外，不额外新增“主观测载体”（例如不依赖新增结构化日志作为主入口）。
- 丢弃计数口径采用“进程生命周期累计”（从进程启动开始累积）。
- `degraded` 状态在静默期后可自动恢复为正常（quiet period 的长度由实现决定，见 Claude's Discretion）。
- `degraded` 状态下至少提供“最近一次 drop 时间”（`last_drop_at`），帮助判断问题是否仍在发生。

### 安全与泄露边界（auth + detail level）
- `degraded` 时允许提供少量诊断细节，但必须避免泄露敏感信息（不返回 request_id 列表、SQL 错误原文、用户输入等）。
- “原因”对外呈现为固定原因码（enum），便于稳定采集/告警（例如 `queue_full` / `insert_failure` / `writer_not_started`）。
- 所有 health/降级信息仅在现有 management auth 边界内可见（仅 management key 可见；其他对外接口不暴露）。
- 允许输出系统级信号帮助定位为“系统问题”，而不是把排查压力推到单个请求上。

### Claude's Discretion
- `quiet period` 的具体长度与恢复规则细节（需在“不过度抖动”与“能恢复正常”之间取平衡）。
- Management API 的具体字段命名与结构（例如 `meta.persistence.{degraded,dropped_total,last_drop_at,last_drop_reason}`）。
- 原因码（enum）集合的最终命名与是否包含 `last_drop_reason`（在“默认最小输出”与“允许少量诊断细节”之间做一致化）。

</decisions>

<specifics>
## Specific Ideas

- 目标用户：运维/管理员（management API 是主入口）。
- 合同风格：best-effort 需要明确到“可丢场景列表”，并把丢行语义定义为 degraded（可观测）。

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within phase scope.

</deferred>

---

## Audit References

Source: `.planning/v1-MILESTONE-AUDIT.md`

- Tech debt (phase 03): enqueue queue-full / writer-not-started / insert failure 等会 drop 或抑制错误。
- Tech debt (phase 06): ensurePublished 强保证与 persistence best-effort 的张力。

---

*Phase: 08-persistence-contract-observability*
*Context gathered: 2026-01-31*
