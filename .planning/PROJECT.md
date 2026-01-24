# CLIProxyAPI - TPS Metrics Milestone

## What This Is

CLI Proxy API 是一个多协议 AI API 代理服务，支持 OpenAI、Gemini、Claude 等多种 API 格式的双向转换。本 milestone 聚焦于增加 tokens per second (TPS) 性能指标监控功能。

## Core Value

**实时可见的 API 响应性能**：用户能够在流式响应过程中看到当前 TPS，请求结束后获得汇总统计，并能查询历史性能数据。

## Requirements

### Validated

<!-- 已有功能，从现有代码推断 -->

- ✓ Multi-protocol API proxy (OpenAI/Gemini/Claude) — existing
- ✓ Provider-agnostic authentication (OAuth + API key) — existing
- ✓ Bidirectional request/response translation — existing
- ✓ Streaming response forwarding — existing
- ✓ Credential rotation and quota management — existing
- ✓ Hot-reload configuration — existing
- ✓ Usage tracking per credential — existing

### Active

<!-- 本 milestone 目标 -->

- [ ] TPS 实时计算：输出 tokens / 生成时间
- [ ] 流式响应中实时显示当前 TPS
- [ ] 响应结束后显示本次请求汇总 TPS
- [ ] TPS 数据写入日志
- [ ] 按 provider/模型分别统计 TPS
- [ ] TPS 数据持久化到数据库
- [ ] 历史 TPS 数据查询接口

### Out of Scope

- 输入 tokens 速度统计 — 本次只关注输出 tokens
- TPS 告警/阈值通知 — 可在后续 milestone 添加
- 可视化仪表盘 — 本次只提供数据和 API

## Context

**现有架构**：
- 分层代理架构，包含 API 层、Handler 层、认证层、翻译层、执行器层
- 已有 usage tracking 基础设施（内存中按 credential 聚合）
- 流式响应通过 Handler 层转发

**TPS 计算切入点**：
- 执行器层（`internal/runtime/executor/`）处理实际的 HTTP 请求/响应
- 流式响应在 Handler 层处理，可在此注入 TPS 计算逻辑
- 现有 usage tracking 可扩展以支持 TPS 指标

## Constraints

- **Tech stack**: Go，与现有代码库保持一致
- **Storage**: 使用嵌入式数据库（如 SQLite），避免引入外部依赖
- **Performance**: TPS 计算不应显著影响响应延迟

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| 仅计算输出 tokens | 用户关注模型生成速度，输入 tokens 处理时间相对固定 | — Pending |
| 使用数据库存储 | 支持历史查询和聚合分析 | — Pending |
| 流式实时显示 | 用户需要在等待过程中看到性能反馈 | — Pending |

---
*Last updated: 2025-01-29 after initialization*
