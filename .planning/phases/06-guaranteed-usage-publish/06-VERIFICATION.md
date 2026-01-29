---
phase: 06-guaranteed-usage-publish
verified: 2026-01-30T15:31:39Z
status: passed
score: 3/3 must-haves verified
---

# Phase 6: Guaranteed Usage Publish Verification Report

**Phase Goal:** 在上游不返回 usage/tokens 的情况下仍保证至少 1 条 usage record 被发布，从而保证 SQLite 有请求行可查
**Verified:** 2026-01-30T15:31:39Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Must-Haves (ROADMAP Phase 6 Success Criteria)

| # | Must-have | Status | Evidence |
|---|-----------|--------|----------|
| 1 | Streaming executor 在结束时调用 `ensurePublished`（即使没有任何 usage chunk） | ✓ VERIFIED | 所有 `ExecuteStream` 实现都在 stream goroutine 结束处调用 `ensurePublished`：`internal/runtime/executor/claude_executor.go:317`、`internal/runtime/executor/codex_executor.go:276`、`internal/runtime/executor/qwen_executor.go:236`、`internal/runtime/executor/gemini_executor.go:296`、`internal/runtime/executor/gemini_cli_executor.go:380`、`internal/runtime/executor/gemini_vertex_executor.go:615`、`internal/runtime/executor/aistudio_executor.go:254`、`internal/runtime/executor/openai_compat_executor.go:287`、`internal/runtime/executor/iflow_executor.go:282`、`internal/runtime/executor/antigravity_executor.go:811` |
| 2 | Failure paths 仍会发布 failure record（不会被 `ensurePublished` 抢占/覆盖） | ✓ VERIFIED | `finalize(ctx, &err)` 先 `trackFailure` 再 `ensurePublished`：`internal/runtime/executor/usage_helpers.go:62`；`publishFailure` 与 `ensurePublished` 共用 `once.Do`，先发布者获胜：`internal/runtime/executor/usage_helpers.go:78`；单测锁定失败优先：`internal/runtime/executor/usage_helpers_test.go:79`、`internal/runtime/executor/usage_helpers_test.go:104` |
| 3 | 测试锁定：无 usage metadata 的请求仍会在 SQLite 中出现可查询行 | ✓ VERIFIED | SQLite 回归测试直接断言 `metrics` 表存在 `request_id` 行且 token/派生指标为 NULL：`internal/metricsruntime/guaranteed_usage_publish_test.go:17` |

**Score:** 3/3 must-haves verified

## Required Artifacts (Exist / Substantive / Wired)

| Artifact | Expected | Status | Details |
|---------|----------|--------|---------|
| `internal/runtime/executor/usage_helpers.go` | `usageReporter.ensurePublished` / failure precedence / once semantics | ✓ VERIFIED | 提供 `ensurePublished` + `finalize`（先失败、后保证发布）并通过 `once.Do` 保证“最多一次发布” |
| `internal/runtime/executor/usage_helpers_test.go` | 单测锁定 no-usage + failure precedence + once semantics | ✓ VERIFIED | 覆盖：no-usage publish 跳过、ensurePublished emits once、failure 优先不被覆盖 |
| `internal/runtime/executor/guaranteed_usage_publish_wiring_test.go` | 约束关键 executor 文件必须存在 hook | ✓ VERIFIED | 对列出的 7 个 executor 文件同时要求：`defer reporter.finalize(ctx, &err)` + `defer reporter.ensurePublished(ctx)` |
| `internal/metricsruntime/guaranteed_usage_publish_test.go` | SQLite 可查询行回归测试 | ✓ VERIFIED | 使用 `metricspersist.InitDB`/`Migrate` 写入并查询 `metrics` 表，断言 no-usage 仍落一行 |
| `internal/runtime/executor/*_executor.go` (streaming) | stream goroutine 结束处保证调用 `ensurePublished` | ✓ VERIFIED | 逐一核对所有 `ExecuteStream`：见 Must-have #1 证据 |

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/runtime/executor/usage_helpers.go` | `sdk/cliproxy/usage/manager.go` | `publishUsageRecord` → `usage.PublishRecord` → `DefaultManager().Publish` | ✓ WIRED | `publishUsageRecord` 默认指向 `usage.PublishRecord`：`internal/runtime/executor/usage_helpers.go:31`；`PublishRecord` 走 default manager：`sdk/cliproxy/usage/manager.go:175` |
| `sdk/cliproxy/usage` | `internal/metricsruntime/usage_plugin.go` | `coreusage.RegisterPlugin(metricsruntime.NewMetricsPlugin(...))` | ✓ WIRED | 插件通过 init 注册：`internal/usage/metrics_plugin.go:9`；被 server/service 引入：`cmd/server/main.go:29` 与 `sdk/cliproxy/service.go:18` |
| `internal/metricsruntime/usage_plugin.go` | SQLite `metrics` 表 | `enqueueMetricRecord(metricspersist.MetricRecord{...})` | ✓ WIRED | usageMissing 时 token 指针保持 nil（落库 NULL）：`internal/metricsruntime/usage_plugin.go:51`；落库行为由测试 `internal/metricsruntime/guaranteed_usage_publish_test.go:75` 验证 |

## Requirements Coverage (REQUIREMENTS.md)

| Requirement | Status | Blocking Issue |
|------------|--------|----------------|
| STOR-01 (SQLite 持久化) | ✓ SATISFIED (strengthened) | None — 本 phase 通过 ensurePublished + SQLite 回归测试补齐“缺 usage 不落库”的可靠性缺口 |
| STOR-02 (REST API 查询) | ✓ SATISFIED (strengthened) | None — 该 phase 确保“可查询行”存在，增强查询的完整性 |
| DISP-02 (结构化日志) | ? OUT OF PHASE | 本 phase 目标是“保证落库一行”，并不直接完成日志要求 |

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/runtime/executor/iflow_executor.go:481` | 481 | `placeholder` literal | ℹ️ Info | 这里是用于稳定上游流式行为的占位工具定义，不属于 Phase 6 的“stub 未实现”风险 |

## Gaps Summary

未发现阻塞 Phase 6 目标达成的缺口：流式/非流式路径均能保证发布至少一条 usage record；失败优先级通过实现与单测锁定；SQLite 侧有回归测试确保无 usage 时仍可查询到 `request_id` 行。

## Recommended Follow-ups

1) 更新 `.planning/ROADMAP.md` Phase 6 的进度/勾选状态，避免“代码已实现但路线图显示 Planned”的漂移。
2) 在 CI/本地回归时优先跑：`go test ./...`（至少包含 `./internal/runtime/executor` 与 `./internal/metricsruntime`）以确保 hook/回归测试持续有效。

---

_Verified: 2026-01-30T15:31:39Z_
_Verifier: Claude (gsd-verifier)_
