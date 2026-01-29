# Phase 2: Metrics Collection - Context

**Gathered:** 2026-01-29
**Status:** Ready for planning

<domain>
## Phase Boundary

将 Phase 1 建立的 TPS 指标计算能力集成到现有流式响应处理流程中。用户在流式响应过程中能看到实时 TPS，响应结束后获得完整指标汇总。同时指标数据以结构化格式写入日志文件。

本阶段不包含：历史数据查询、数据持久化到数据库、指标告警/阈值功能。

</domain>

<decisions>
## Implementation Decisions

### 指标显示时机
- **流式中显示 + 结束后汇总**：两者都要
- **更新频率**：每秒更新一次当前 TPS
- **显示位置**：同一行覆盖（使用 `\r`），不滚动屏幕
- **汇总内容**：完整版 — TPS、TTFT、TPOT、总 token、耗时等全部指标

### 日志格式与存储
- **日志位置**：写入文件（如 `logs/metrics-YYYY-MM-DD.jsonl`）
- **日志格式**：JSON Lines（每行一个 JSON 对象）
- **日志字段**：全部字段 — tracking_id, provider, model, tps, ttft, tpot, input_tokens, output_tokens, duration_ms, request_path, status_code, error_info, timestamp
- **日志轮转**：按日期轮转（每天一个新文件）

### 性能与并发
- **指标收集方式**：异步收集 — 通过 channel/队列发送到后台处理，不阻塞响应流
- **日志写入**：异步写入 — 使用 buffered channel，后台 goroutine 批量写入
- **错误处理**：静默丢弃 — 日志写入失败不影响主流程
- **背压处理**：始终完整收集 — 不降级，保证数据完整性

### 跨 Provider 一致性
- **Token 来源**：使用 provider 返回的 usage 字段（total_tokens / completion_tokens）
- **响应格式处理**：在现有翻译层处理 — 复用协议转换层，在转换时提取指标
- **Chunk 格式**：Provider 特定解析 — 每个 provider 有自己的 chunk 解析逻辑
- **缺失指标**：标记为 null — 日志中该字段为 null 表示不可用

### Claude's Discretion
- 异步队列的具体实现（channel buffer 大小、worker 数量）
- 实时 TPS 显示的具体格式和样式
- 日志文件路径配置方式
- 后台 goroutine 的生命周期管理

</decisions>

<specifics>
## Specific Ideas

- 流式响应中的 TPS 显示应该类似下载进度条的感觉
- 日志文件按日期命名便于后续分析和归档
- 指标收集是旁路功能，绝不能影响主响应流程

</specifics>

<deferred>
## Deferred Ideas

- 历史数据查询 API — Phase 4
- 数据持久化到 SQLite — Phase 3
- 指标告警/阈值通知 — 后续 milestone
- 可视化仪表盘 — 后续 milestone

</deferred>

---

*Phase: 02-metrics-collection*
*Context gathered: 2026-01-29*
