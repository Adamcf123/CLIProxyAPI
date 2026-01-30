---
phase: 08-persistence-contract-observability
verified: 2026-01-30T18:15:57Z
status: passed
score: 5/5 must-haves verified
---

# Phase 8: Persistence Contract & Observability Verification Report

**Phase Goal:** 明确并固化“best-effort 持久化”的语义契约，补齐可观测性以避免静默缺行。
**Verified:** 2026-01-30T18:15:57Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

从业务结果倒推，本阶段要让“允许丢行”不再等于“静默缺行”：当 best-effort 持久化发生 drop/失败时，系统必须在管理查询中给出可聚合、可定位且不泄露敏感信息的 degraded 信号；并且在健康状态下，管理接口默认 JSON 必须不漂移；此外 degraded 必须能在静默期后自动恢复；writer 在启动阶段遇到 schema 不可用必须 fail-fast；最后这些语义要被测试与文档锁死。

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | 运行时 drop/失败（queue_full / writer_not_started / insert_failure）会形成可查询 degraded 信号（计数+时间+可选原因），且不泄露 request_id 列表 / SQL error 原文 / 用户输入 | ✓ VERIFIED | 进程级 health 与稳定原因码：`internal/metricspersist/health.go:8`、`internal/metricspersist/health.go:13`、`internal/metricspersist/health.go:26`；writer 丢弃点记录原因（无 error/string 拼接）：`internal/metricspersist/writer.go:95`、`internal/metricspersist/writer.go:99`、`internal/metricspersist/writer.go:104`、`internal/metricspersist/writer.go:137`、`internal/metricspersist/writer.go:155`；management 输出只包含最小字段：`internal/api/handlers/management/metrics.go:36`、`internal/api/handlers/management/metrics.go:277` |
| 2 | `/v0/management/metrics` 默认 JSON 输出不变：仅当 degraded 才出现 `meta.persistence`；健康时必须缺失该字段 | ✓ VERIFIED | `metricsMeta.Persistence` 为指针 + `omitempty`：`internal/api/handlers/management/metrics.go:22`；仅在 degraded 且有 last_drop_at 时赋值：`internal/api/handlers/management/metrics.go:277`；契约测试覆盖 omitted vs emitted：`internal/api/handlers/management/metrics_persistence_test.go:15`、`internal/api/handlers/management/metrics_persistence_test.go:48`；外部 test 契约也锁定：`test/metrics_management_test.go:434` |
| 3 | degraded 自动恢复存在（quiet period 常量，文档与实现一致） | ✓ VERIFIED | quiet period 常量：`internal/metricspersist/health.go:33`；degraded 计算逻辑：`internal/metricspersist/health.go:92`；单测锁定恢复：`internal/metricspersist/health_test.go:9`、`internal/metricspersist/writer_test.go:188`；README 文档明确 5m：`README.md:86` |
| 4 | writer 启动时若 schema 不可用导致 Prepare 失败，会返回 error（支持启动链路 fail-fast），不会在 goroutine 内静默退出 | ✓ VERIFIED | Start 内 preflight Prepare，失败直接 return error：`internal/metricspersist/writer.go:66`、`internal/metricspersist/writer.go:74`、`internal/metricspersist/writer.go:112`；单测锁定缺 schema 时 Start 失败：`internal/metricspersist/writer_test.go:101`、`internal/metricspersist/health_test.go:111`；启动链路显式 fail-fast：`cmd/server/main.go:502` |
| 5 | 测试存在且锁定契约：unit tests + contract tests | ✓ VERIFIED | unit（drop reasons + quiet period + start preflight）：`internal/metricspersist/writer_test.go:74`、`internal/metricspersist/writer_test.go:118`、`internal/metricspersist/writer_test.go:148`、`internal/metricspersist/writer_test.go:188`；contract（meta.persistence 仅 degraded 出现 + 字段最小集合）：`internal/api/handlers/management/metrics_persistence_test.go:15`、`internal/api/handlers/management/metrics_persistence_test.go:48`、`test/metrics_management_test.go:434`、`test/metrics_management_test.go:461` |

**Score:** 5/5 truths verified

## Required Artifacts

| Artifact | Expected | Status | Details |
|---------|----------|--------|---------|
| `internal/metricspersist/health.go` | 进程级 health（DroppedTotal/LastDropAt/LastDropReason + quiet period degraded）+ 稳定 enum 且不泄露敏感信息 | ✓ VERIFIED | 存在原子计数器与静默期逻辑（`internal/metricspersist/health.go:45`、`internal/metricspersist/health.go:92`），并在类型注释中明确禁止敏感信息（`internal/metricspersist/health.go:8`） |
| `internal/metricspersist/writer.go` | drop 点记录 reason；Start 预检 Prepare fail-fast | ✓ VERIFIED | Enqueue drop（`internal/metricspersist/writer.go:95`），run Prepare/Exec 失败记录 insert_failure（`internal/metricspersist/writer.go:137`、`internal/metricspersist/writer.go:171`），Start preflight（`internal/metricspersist/writer.go:74`） |
| `internal/api/handlers/management/metrics.go` | 仅 degraded 时输出 `meta.persistence`（默认 JSON 不变） | ✓ VERIFIED | `omitempty` + degraded gating（`internal/api/handlers/management/metrics.go:22`、`internal/api/handlers/management/metrics.go:277`） |
| `internal/api/handlers/management/handler.go` | 支持构造时注入 health provider（避免运行时可变） | ✓ VERIFIED | 默认 provider 指向 `metricspersist.GetPersistenceHealth`（`internal/api/handlers/management/handler.go:79`），并提供构造期 option（`internal/api/handlers/management/handler.go:70`） |
| `internal/api/handlers/management/metrics_persistence_test.go` | 管理 API 合同测试（omit vs emit + 最小字段集合） | ✓ VERIFIED | 覆盖 percentiles 模式下 omitted / emitted（`internal/api/handlers/management/metrics_persistence_test.go:15`、`internal/api/handlers/management/metrics_persistence_test.go:48`） |
| `internal/metricspersist/writer_test.go` + `internal/metricspersist/health_test.go` | unit tests（drop reasons + quiet period + Start preflight） | ✓ VERIFIED | 覆盖 writer_not_started / queue_full / insert_failure + quiet period（见上） |
| `test/metrics_management_test.go` | 外部契约测试（二次锁定 meta.persistence 的出现/缺失与最小字段集合） | ✓ VERIFIED | 使用 handler option 注入 health 并断言 JSON map（`test/metrics_management_test.go:434`、`test/metrics_management_test.go:461`） |
| `README.md` | 对外契约文本（droppable enum + observability + quiet period 值） | ✓ VERIFIED | 明确三类原因码与 5m 恢复语义（`README.md:86`） |

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/metricspersist/writer.go` | `internal/metricspersist/health.go` | `recordPersistenceDrop(DropReason*, time.Now().UTC())` | ✓ WIRED | enqueue 未启动/队列满，以及 run Prepare/Exec 错误都会记录 drop（`internal/metricspersist/writer.go:99`、`internal/metricspersist/writer.go:104`、`internal/metricspersist/writer.go:137`、`internal/metricspersist/writer.go:171`） |
| `internal/api/handlers/management/metrics.go` | `internal/api/handlers/management/handler.go` | `h.persistenceHealth(h.nowUTC())` | ✓ WIRED | management handler 通过构造期注入 provider 获取 health，并决定是否输出 `meta.persistence`（`internal/api/handlers/management/metrics.go:277`、`internal/api/handlers/management/handler.go:70`、`internal/api/handlers/management/handler.go:79`） |
| `cmd/server/main.go` | `internal/metricspersist/writer.go` | `metricspersist.StartWriter(db)` error -> `os.Exit(1)` | ✓ WIRED | 启动阶段若 writer Start 返回 error，会 fail-fast 退出（`cmd/server/main.go:502`） |

## Requirements Coverage

本阶段 ROADMAP 标注为 hardening（无 REQUIREMENTS.md 映射项）。

## Anti-Patterns Found

未发现阻断性 stub/placeholder/TODO 模式（在相关目录的常见 stub 关键字扫描未命中）。

## Human Verification Required

无（本阶段目标的关键语义契约均已由代码结构 + 测试锁定）。

## Notes / Residual Risks (Non-blocking)

- `sqliteWriter.run()` 在启动后如果发生 `Prepare` 失败会记录 `insert_failure` 并退出 goroutine（`internal/metricspersist/writer.go:137`）。这不会“静默缺行”（会进入 degraded），但意味着 writer 不会自愈重启；后续 enqueue 会逐步触发 `queue_full` drop。此行为是否符合长期运行预期，需要产品/运维侧确认是否要引入可控的重启策略（不在本 phase must-haves 内）。

---

_Verified: 2026-01-30T18:15:57Z_
_Verifier: Claude (gsd-verifier)_
