# Phase 10: Request ID Robustness - Context

**Gathered:** 2026-02-01
**Status:** Ready for planning

<domain>
## Phase Boundary

强化 request_id 唯一性与冲突处理，使碰撞不会表现为"静默缺行"。

本阶段要解决的核心问题：当前 cliproxy 使用 32-bit request_id（8-character hex），配合 SQLite `ON CONFLICT(request_id) DO NOTHING` 去重；当碰撞发生时，用户查询会"查不到行"而没有错误提示。

交付结果必须满足：
1. request_id 冲突概率显著降低（通过扩大 ID 空间）
2. 冲突可被检测/暴露（通过日志/监控）
3. 冲突不再表现为"静默缺行"的用户体验

不在本阶段范围内：
- 修改 request_id 的生成位置或传播方式
- 引入分布式 ID 生成器（如 Snowflake）
- 修改 DB schema 添加冲突处理列

</domain>

<decisions>
## Implementation Decisions

### request_id 来源
- **cliproxy 原有代码**：`internal/logging/requestid.go:GenerateRequestID()` 生成 8-character hex（32-bit）
- 本 milestone 只是使用该 ID 作为 metrics 表的主键，未修改生成逻辑

### 冲突检测策略
- **选项 A：扩大 ID 空间让碰撞概率可忽略**
- 从 32-bit 升级到 64-bit 或 UUID（128-bit）
- 不保留显式检测逻辑，依赖概率降低

### 冲突暴露方式
- **选项 A：Query API 返回特殊错误码/消息**
- 当查询的 request_id 不存在时，区分"从未写入" vs "写入被冲突丢弃"
- 需要某种机制追踪冲突事件

### 数据库处理策略
- **选项 C：保持 `ON CONFLICT DO NOTHING` 静默，但增加冲突计数/日志**
- 不修改 DB 冲突处理行为（避免影响性能/复杂度）
- 在 writer 层检测冲突（通过影响行数）并记录到 health/metrics
- 冲突事件可通过 `/v0/management/metrics` 的 meta 暴露

### Claude's Discretion
- 具体升级到 64-bit 还是 UUID（权衡可读性 vs 碰撞概率）
- 冲突日志的具体格式和位置
- Query API 区分"不存在" vs "被冲突丢弃"的具体实现方式

</decisions>

<specifics>
## Specific Ideas

- 当前：`GenerateRequestID()` 使用 `crypto/rand` 生成 4 bytes → 8 hex chars
- 目标：保持 hex 格式便于阅读，但增加长度（如 16 chars = 64-bit）
- 冲突检测：SQLite `ON CONFLICT DO NOTHING` 返回的影响行数为 0 时可判定为冲突
- 向后兼容：现有数据保持 8-char，新数据使用更长格式；查询时兼容两种长度

</specifics>

<deferred>
## Deferred Ideas

- 分布式 ID 生成器（Snowflake、ULID 等）— 超出本 milestone 范围
- 冲突时自动重试生成新 ID — 可能改变请求追踪语义
- 添加独立的冲突审计表 — 增加复杂度，当前通过日志/health 暴露足够

</deferred>

---

*Phase: 10-request-id-robustness*
*Context gathered: 2026-02-01*
