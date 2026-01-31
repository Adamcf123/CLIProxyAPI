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
- [x] **Phase 5: Streaming Failure Semantics** - 流式失败语义可追溯且不会污染聚合
- [x] **Phase 6: Guaranteed Usage Publish** - 无 usage 场景也能落库，保证历史可追溯
- [x] **Phase 7: Docs & Traceability Cleanup** - 修复规划文档漂移（Requirements/Docs 与实现一致）
- [x] **Phase 8: Persistence Contract & Observability** - 明确 best-effort 持久化契约并补齐可观测性
- [x] **Phase 9: Cancel/Disconnect Semantics** - 明确客户端取消/断连的失败语义并锁定测试
- [ ] **Phase 10: Request ID Robustness** - 强化 request_id 唯一性与冲突可见性，避免静默缺行
- [ ] **Phase 11: Runtime Validation (Optional)** - 在真实流量下验证性能/输出/极端流式错误语义

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

**Plans**: 4 plans in 1 wave

Plans:
- [x] 05-01-PLAN.md — 在 ForwardStream 中写入可持久化失败信号（RequestState.LastError）+ 单测
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

**Plans**: 5 plans in 3 waves

Plans:
- [x] 06-01-PLAN.md — usageReporter: 发布测试缝 + 失败优先级单测
- [x] 06-02-PLAN.md — Claude/Codex/Qwen: 结束时 ensurePublished（含流式 goroutine）
- [x] 06-03-PLAN.md — Gemini/Vertex/AIStudio: 结束时 ensurePublished（含流式 goroutine）
- [x] 06-04-PLAN.md — SQLite 回归测试：无 usage 也必须可查
- [x] 06-05-PLAN.md — Executor wiring 回归：stream end 必须 ensurePublished

### Phase 7: Docs & Traceability Cleanup

**Goal**: 修复规划文档漂移，确保 requirements/traceability/docs 与已验证实现一致

**Depends on**: Phase 6

**Requirements**: (none — tech debt closure)

**Gap Closure:** Closes audit tech debt about REQUIREMENTS.md traceability drift and legacy docs referencing JSONL.

**Success Criteria** (what must be TRUE):
  1. `REQUIREMENTS.md` 的 traceability/checklist 与 Phase 1 verification 的结论一致（METR-01..04 标记为 satisfied/complete）
  2. 文档/重要命令不再引用 legacy JSONL 作为数据源（明确 SQLite 是单一来源）
  3. 变更有最小化范围且可被审计复核（清晰的变更点与理由）

**Plans**: 4 plans in 1 wave

Plans:
- [x] 07-01-PLAN.md — 对齐 REQUIREMENTS.md（METR-01..04）到 Phase 01 验证结论
- [x] 07-02-PLAN.md — 补齐 README 的 SQLite/Query API 指南 + 修复 MetricsPlugin 注释漂移
- [x] 07-03-PLAN.md — 修复 STATE.md + operator docs 的 JSONL 漂移（对齐 SQLite 单一来源）
- [x] 07-04-PLAN.md — 清理 docs/*.md 的 JSONL 遗留表述（对齐 SQLite 单一来源）

### Phase 8: Persistence Contract & Observability

**Goal**: 明确并固化“best-effort 持久化”的语义契约，补齐可观测性以避免静默缺行

**Depends on**: Phase 7

**Requirements**: (none — hardening)

**Gap Closure:** Closes audit tech debt about best-effort persistence drops and ensurePublished vs persistence guarantee tension.

**Success Criteria** (what must be TRUE):
  1. 对外可见的语义契约明确：哪些场景允许丢行、如何被观测到、如何被用户理解
  2. 关键丢弃路径可追踪（例如 queue-full / writer-not-started / insert failure）且不会弱化安全边界
  3. 关键行为有回归测试或契约测试锁定（至少覆盖可观测性信号）

**Plans**: 2 plans in 2 waves

Plans:
- [x] 08-01-PLAN.md — 在 writer drop/insert failure 路径记录 health，并仅在 degraded 时向 management meta 暴露
- [x] 08-02-PLAN.md — 用单测/契约测试锁定 degraded meta 语义，并在 README 固化 best-effort 契约与示例

### Phase 9: Cancel/Disconnect Semantics

**Goal**: 明确客户端取消/断连的归类语义（success/failure/canceled），并锁定 Query API 分类

**Depends on**: Phase 8

**Requirements**: (none — hardening)

**Gap Closure:** Closes audit tech debt about client cancel/disconnect potentially being classified as success.

**Success Criteria** (what must be TRUE):
  1. 取消/断连的系统语义明确且一致（写入层、查询层、聚合层对齐）
  2. Query API 在取消/断连场景不会把 canceled 误归类为 success
  3. 新增测试锁定该语义（避免回归）

**Plans**: 3 plans in 3 waves

Plans:
- [x] 09-01-PLAN.md — Status Code 499 语义：将客户端取消/断连映射到 499 状态码
- [x] 09-02-PLAN.md — Query API Canceled 三分法：success/failure/canceled 显式区分
- [x] 09-03-PLAN.md — 测试锁定：写入层与 non-streaming 断连归类测试 + 全量回归 gate

### Phase 10: Request ID Robustness

**Goal**: 强化 request_id 唯一性与冲突处理，使碰撞不会表现为“静默缺行”

**Depends on**: Phase 9

**Requirements**: (none — reliability)

**Gap Closure:** Closes audit tech debt about short (32-bit) request_id collisions + ON CONFLICT DO NOTHING masking missing rows.

**Success Criteria** (what must be TRUE):
  1. request_id 的冲突概率显著降低，或冲突在系统内可被明确检测/暴露
  2. 冲突不会悄悄变成“查不到行”的用户体验（可观测/可诊断/可解释）
  3. 新增测试或属性验证覆盖至少一种冲突/重复路径

**Plans**: 3 plans in 3 waves

Plans:
- [x] 10-01-PLAN.md — 升级 request_id 生成器到 64-bit (16-char hex)
- [x] 10-02-PLAN.md — 在 writer 层检测 request_id 冲突并记录到 health
- [ ] 10-03-PLAN.md — 测试锁定冲突检测与暴露链路

### Phase 11: Runtime Validation (Optional)

**Goal**: 在真实环境/压测中验证关键 SLO 与边界语义，给发布前信心

**Depends on**: Phase 10

**Requirements**: (none — validation)

**Gap Closure:** Closes audit recommendations for runtime/human verification (non-blocking, output correctness, hard-to-prove streaming errors).

**Success Criteria** (what must be TRUE):
  1. metrics collection 的非阻塞性在真实负载下被验证（无明显吞吐/延迟退化）
  2. stderr 实时输出/汇总与落库在真实环境权限/部署方式下验证可用
  3. 对“headers 已提交后发生 terminal error”等难以单测覆盖的边界做过实测并记录结论

**Plans**: TBD (not planned yet)

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Metrics Foundation | 4/4 | Complete | 2025-01-29 |
| 2. Metrics Collection | 4/4 | Complete | 2026-01-29 |
| 3. Persistence | 3/3 | Complete | 2026-01-30 |
| 4. Query API | 4/4 | Complete | 2026-01-30 |
| 5. Streaming Failure Semantics | 3/3 | Complete | 2026-01-30 |
| 6. Guaranteed Usage Publish | 5/5 | Complete | 2026-01-30 |
| 7. Docs & Traceability Cleanup | 4/4 | Complete | 2026-01-31 |
| 8. Persistence Contract & Observability | 2/2 | Complete | 2026-01-30 |
| 9. Cancel/Disconnect Semantics | 3/3 | Complete | 2026-02-01 |
| 10. Request ID Robustness | 2/3 | In progress | |
| 11. Runtime Validation (Optional) | 0/? | Planned (Optional) | |
