# Roadmap: CLIProxyAPI TPS Metrics

## Overview

从建立核心指标计算逻辑开始，逐步集成到流式响应处理中，然后添加持久化存储，最后提供历史数据查询和分析能力。每个阶段交付完整可验证的功能，确保用户能够逐步获得 TPS 指标的可见性。

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Metrics Foundation** - 建立核心指标计算逻辑和数据结构
- [x] **Phase 2: Metrics Collection** - 集成指标收集到流式响应处理
- [x] **Phase 3: Persistence** - SQLite 持久化存储
- [x] **Phase 4: Query API** - 历史指标查询和分析接口
- [ ] **Phase 5: Streaming Failure Semantics** - 流式失败语义可追溯且不会污染聚合
- [ ] **Phase 6: Guaranteed Usage Publish** - 无 usage 场景也能落库，保证历史可追溯

## Phase Details

### Phase 1: Metrics Foundation

**Goal**: 建立完整的指标计算能力，支持 TPS、TTFT、TPOT 计算，并按 provider/model 分组统计

**Depends on**: Nothing (first phase)

**Requirements**: METR-01, METR-02, METR-03, METR-04

**Success Criteria** (what must be TRUE):
  1. 系统能够正确计算 TPS（输出 tokens / 生成时间）
  2. 系统能够正确计算 TTFT（首个 token 响应时间）
  3. 系统能够正确计算 TPOT（每个输出 token 的平均时间）
  4. 指标能够按 provider 和 model 分别统计和聚合
  5. 每个请求生成唯一的 tracking ID 用于指标关联

**Plans**: 4 plans in 2 waves

Plans:
- [x] 01-01-PLAN.md — 设计和实现 TPSCollector 核心结构和数据类型
- [x] 01-02-PLAN.md — 实现 TPS、TTFT、TPOT 计算逻辑和单元测试
- [x] 01-03-PLAN.md — 实现滑动窗口聚合和百分位统计
- [x] 01-04-PLAN.md — 完善 TPSCollector 集成滑动窗口和计算功能

### Phase 2: Metrics Collection

**Goal**: 将指标收集集成到流式响应处理流程中，确保响应后显示指标汇总并写入结构化日志

**Depends on**: Phase 1

**Requirements**: DISP-01, DISP-02

**Success Criteria** (what must be TRUE):
  1. 响应结束后用户能够看到本次请求的指标汇总（包括 TPS、TTFT、TPOT）
  2. 指标数据以结构化格式写入日志文件
  3. 指标收集不影响流式响应的延迟和吞吐量
  4. 跨不同 provider（OpenAI、Gemini、Claude）的指标收集正常工作

**Plans**: 4 plans in 2 waves (including 1 gap closure)

Plans:
- [x] 02-01-PLAN.md — 打牢采集基础（TTFT hook + collector 并发安全 + 单请求 state）
- [x] 02-02-PLAN.md — 流式中每秒显示 + 结束后指标汇总（stderr）
- [x] 02-03-PLAN.md — JSONL 按日落盘（logs/metrics-YYYY-MM-DD.jsonl）
- [x] 02-04-PLAN.md — 修复 TTFT 采样缺口（OpenAI/Gemini/Claude 首 chunk 路径收敛）

### Phase 3: Persistence

**Goal**: 使用 SQLite 持久化存储指标数据，确保历史数据可追溯

**Depends on**: Phase 2

**Requirements**: STOR-01

**Success Criteria** (what must be TRUE):
  1. 指标数据成功写入 SQLite 数据库
  2. 数据库 schema 支持指标的所有字段（TPS、TTFT、TPOT、provider、model、timestamp 等）
  3. 数据库写入不影响响应性能
  4. 数据库文件正确初始化和迁移

**Plans**: 3 plans in 3 waves

Plans:
- [x] 03-01-PLAN.md — SQLite schema + migrations wiring
- [x] 03-02-PLAN.md — Async SQLite writer + MetricsPlugin integration
- [x] 03-03-PLAN.md — Retention cleanup + disable JSONL legacy

### Phase 4: Query API

**Goal**: 提供 REST API 查询历史指标数据，支持百分位统计和时间窗口聚合

**Depends on**: Phase 3

**Requirements**: STOR-02, STOR-03, STOR-04

**Success Criteria** (what must be TRUE):
  1. 用户能够通过 REST API 查询历史指标数据
  2. API 支持 p50、p95、p99 百分位统计查询
  3. API 支持按时间窗口（如 1 小时、1 天）聚合查询
  4. API 返回格式清晰、易于解析

**Plans**: 4 plans in 4 waves

Plans:
- [x] 04-01-PLAN.md — 补齐 streaming 维度（schema + writer 入库）
- [x] 04-02-PLAN.md — 上线 GET /v0/management/metrics（request_id 查询 + 校验 + meta）
- [x] 04-03-PLAN.md — 实现 mode=percentiles（p50/p95/p99，复用线性插值语义）
- [x] 04-04-PLAN.md — 实现 mode=buckets（固定粒度 + UTC 对齐 + 空 bucket 回填）

### Phase 5: Streaming Failure Semantics

**Goal**: 修复流式请求的失败语义，使失败能够被可靠落库并在 Query API 中正确归类（不污染 success 聚合与百分位）

**Depends on**: Phase 4

**Requirements**: DISP-01, STOR-02, STOR-03, STOR-04

**Gap Closure:** Closes milestone audit gaps about streaming terminal errors being persisted/classified as success.

**Success Criteria** (what must be TRUE):
  1. OpenAI/Gemini 等流式 terminal error 在写出错误 payload 前会设置非 2xx HTTP status
  2. RequestState 会记录可持久化的 error_info（或等价失败信号），确保 DB 行可判定为 failure
  3. Query API 的 success/failure 切分在“流式失败”场景下准确（新增测试锁定）

**Plans**: 3 plans in 1 wave

Plans:
- [ ] 05-01-PLAN.md — 在 ForwardStream 中写入可持久化失败信号（RequestState.LastError）+ 单测
- [x] 05-02-PLAN.md — 对齐 OpenAI/Gemini terminal error：写错误 payload 前设置非 2xx status
- [x] 05-03-PLAN.md — Query API buckets 回归测试：200 + error_info 必须归类为 failure

### Phase 6: Guaranteed Usage Publish

**Goal**: 在上游不返回 usage/tokens 的情况下仍保证至少 1 条 usage record 被发布，从而保证 SQLite 有请求行可查

**Depends on**: Phase 5

**Requirements**: DISP-02, STOR-01, STOR-02

**Gap Closure:** Closes milestone audit gaps where no-usage paths can produce no persistence row.

**Success Criteria** (what must be TRUE):
  1. Streaming executor 在结束时调用 `ensurePublished`，即使没有任何 usage chunk 也会发布 usage record
  2. Failure paths 仍会发布 failure record（不会被 ensurePublished 抢占）
  3. 新增测试锁定：无 usage metadata 的请求仍会在 SQLite 中出现可查询行

**Plans**: Pending (will be created by `/gsd-plan-phase 6`)

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Metrics Foundation | 4/4 | Complete | 2025-01-29 |
| 2. Metrics Collection | 4/4 | Complete | 2026-01-29 |
| 3. Persistence | 3/3 | Complete | 2026-01-30 |
| 4. Query API | 4/4 | Complete | 2026-01-30 |
| 5. Streaming Failure Semantics | 1/3 | In progress | 2026-01-30 |
| 6. Guaranteed Usage Publish | 0/? | Planned | - |
