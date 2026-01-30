# Requirements: CLIProxyAPI TPS Metrics

**Defined:** 2025-01-29
**Core Value:** 实时可见的 API 响应性能 — 用户能够获得 TPS 指标汇总并查询历史性能数据

## v1 Requirements

### Metrics（指标）

- [x] **METR-01**: 计算 TPS（输出 tokens / 生成时间）
- [x] **METR-02**: 计算 TTFT（首个 token 响应时间）
- [x] **METR-03**: 计算 TPOT（每个输出 token 的平均时间）
- [x] **METR-04**: 按 provider/model 分别统计指标

### Display（展示）

- [x] **DISP-01**: 响应结束后显示本次请求的指标汇总
- [x] **DISP-02**: 将指标写入结构化日志

### Storage（存储）

- [x] **STOR-01**: 使用 SQLite 持久化存储指标数据
- [x] **STOR-02**: 提供 REST API 查询历史指标
- [x] **STOR-03**: 支持百分位统计（p50, p95, p99）
- [x] **STOR-04**: 支持按时间窗口聚合查询

## v2 Requirements

### Real-Time Display（实时展示）

- **RTDP-01**: 流式响应中实时显示当前 TPS
- **RTDP-02**: SSE 端点推送实时指标更新

### Advanced Analytics（高级分析）

- **ANAL-01**: 异常检测和告警
- **ANAL-02**: 成本归因分析
- **ANAL-03**: 可视化仪表盘

## Out of Scope

| Feature | Reason |
|---------|--------|
| 流式实时 TPS 显示 | 用户选择先实现响应后汇总，实时显示延迟到 v2 |
| Token 计数（输入/输出）独立展示 | 本次聚焦 TPS 相关指标，token 计数作为内部计算使用 |
| 告警/阈值通知 | 可在后续 milestone 添加 |
| 可视化仪表盘 | 本次只提供数据和 API |
| Prometheus 导出 | 当前使用 SQLite，Prometheus 集成延迟到 v2 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| METR-01 | Phase 1 | Complete |
| METR-02 | Phase 1 | Complete |
| METR-03 | Phase 1 | Complete |
| METR-04 | Phase 1 | Complete |
| DISP-01 | Phase 2 | Complete |
| DISP-02 | Phase 2 | Complete |
| STOR-01 | Phase 3 | Complete |
| STOR-02 | Phase 4 | Complete |
| STOR-03 | Phase 4 | Complete |
| STOR-04 | Phase 4 | Complete |

**Coverage:**
- v1 requirements: 10 total
- Mapped to phases: 10 ✓
- Unmapped: 0

---
*Requirements defined: 2025-01-29*
*Last updated: 2026-01-31 after phase 7 docs cleanup*
