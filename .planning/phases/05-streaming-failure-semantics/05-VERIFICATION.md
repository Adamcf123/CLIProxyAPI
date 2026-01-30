---
phase: 05-streaming-failure-semantics
verified: 2026-01-30T11:03:15Z
status: passed
score: 3/3 must-haves verified
---

# Phase 5: Streaming Failure Semantics Verification Report

**Phase Goal:** 修复流式请求的失败语义，使失败能够被可靠落库并在 Query API 中正确归类（不污染 success 聚合与百分位）
**Verified:** 2026-01-30T11:03:15Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

本阶段的业务目标是：当流式请求在“已经开始输出 body（headers 可能已提交）”后发生 terminal error 时，系统仍能可靠地把该请求记为失败（可持久化、可查询、可用于聚合/分桶/百分位的 success/failure 切分），并且不会把失败混入 success 聚合。

从代码结构看，闭环由三段组成：
- 失败信号写入（`RequestState.LastError`）
- 写库映射（`LastError` → `MetricRecord.ErrorInfo`，空值不写指针）
- Query API 切分规则（2xx 但 `error_info` 非空也必须归类为 failure）

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Streaming terminal errors set `RequestState.LastError` and are mapped into `MetricRecord.ErrorInfo` for persistence/classification | ✓ VERIFIED | `sdk/api/handlers/stream_forwarder.go:14`（写入 LastError）；`internal/metricsruntime/usage_plugin.go:103`（LastError→ErrorInfo）；测试：`sdk/api/handlers/stream_forwarder_test.go:15`、`internal/metricsruntime/usage_plugin_test.go:15` |
| 2 | OpenAI/Gemini terminal error writer calls `c.Status(status)` before writing the terminal error payload (best-effort when headers not committed) | ✓ VERIFIED | OpenAI chat：`sdk/api/handlers/openai/openai_handlers.go:695`；OpenAI responses：`sdk/api/handlers/openai/openai_responses_handlers.go:230`；Gemini：`sdk/api/handlers/gemini/gemini_handlers.go:343`；Gemini CLI：`sdk/api/handlers/gemini/gemini-cli_handlers.go:231`；测试：`sdk/api/handlers/openai/openai_handlers_test.go:15`、`sdk/api/handlers/gemini/gemini_handlers_test.go:15` |
| 3 | Query API buckets mode classifies `status_code=200 + error_info!=empty` as failure (does not pollute success buckets) | ✓ VERIFIED | buckets SQL success_flag：`internal/api/handlers/management/metrics.go:506`；buckets 回归测试：`test/metrics_management_test.go:605` |

**Score:** 3/3 truths verified

## Required Artifacts

| Artifact | Expected | Status | Details |
|---------|----------|--------|---------|
| `sdk/api/handlers/stream_forwarder.go` | `ForwardStream` records terminal errors into `RequestState.LastError` (before terminal payload/cancel) | ✓ VERIFIED | `maybeSetStreamingTerminalLastError` 在调用 `WriteTerminalError` 之前执行（`sdk/api/handlers/stream_forwarder.go:33`、`sdk/api/handlers/stream_forwarder.go:57`），且在 `cancel(...)` 之前执行（同段逻辑） |
| `sdk/api/handlers/stream_forwarder_test.go` | Regression tests for terminal error → `LastError` | ✓ VERIFIED | 覆盖：`terminal error`（boom）、`no error`（空）、`Error==nil`（使用 status text）（`sdk/api/handlers/stream_forwarder_test.go:15`、`:74`、`:128`） |
| `internal/metricsruntime/usage_plugin.go` | MetricsPlugin maps `RequestStateSnapshot.LastError` → `MetricRecord.ErrorInfo` and keeps it nil when empty | ✓ VERIFIED | `snap.LastError != ""` 才设置 `errorInfoPtr`（`internal/metricsruntime/usage_plugin.go:103`）；空值保留 nil 指针（避免写入空字符串指针） |
| `internal/metricsruntime/usage_plugin_test.go` | Regression test for `LastError` → `ErrorInfo` mapping | ✓ VERIFIED | 通过 test seam 捕获 `MetricRecord` 并断言 `ErrorInfo` 指针语义（`internal/metricsruntime/usage_plugin_test.go:15`） |
| `sdk/api/handlers/openai/openai_handlers.go` | WriteTerminalError sets status before writing SSE error payload | ✓ VERIFIED | `c.Status(status)` 在 `fmt.Fprintf(c.Writer, ...)` 之前执行（`sdk/api/handlers/openai/openai_handlers.go:695`） |
| `sdk/api/handlers/openai/openai_responses_handlers.go` | WriteTerminalError sets status before writing Responses SSE error payload | ✓ VERIFIED | `c.Status(status)` 在 `fmt.Fprintf(c.Writer, ...)` 之前执行（`sdk/api/handlers/openai/openai_responses_handlers.go:230`） |
| `sdk/api/handlers/gemini/gemini_handlers.go` | WriteTerminalError sets status before writing SSE/alt payload | ✓ VERIFIED | `c.Status(status)` 在 SSE 分支 `fmt.Fprintf` / alt 分支 `Writer.Write` 之前执行（`sdk/api/handlers/gemini/gemini_handlers.go:343`） |
| `sdk/api/handlers/gemini/gemini-cli_handlers.go` | WriteTerminalError sets status before writing SSE/alt payload | ✓ VERIFIED | `c.Status(status)` 在 payload 写出前执行（`sdk/api/handlers/gemini/gemini-cli_handlers.go:231`） |
| `test/metrics_management_test.go` | Regression coverage for bucket classification of 200+error_info failures | ✓ VERIFIED | seed 加入 `status=200 && error_info="boom"` failure 行（`test/metrics_management_test.go:275`）；断言 failure bucket count=2 且 success 不受影响（`test/metrics_management_test.go:687`） |

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `sdk/api/handlers/stream_forwarder.go` | `internal/metricsruntime/request_state.go` | `metricsruntime.GetRequestState(c)` + `state.SetLastError(...)` | ✓ WIRED | `GetRequestState` 取 state（`sdk/api/handlers/stream_forwarder.go:18`）并写入 LastError（`sdk/api/handlers/stream_forwarder.go:28`、`:39`、`:44`） |
| `internal/metricsruntime/usage_plugin.go` | `internal/metricspersist/types.go` | `MetricRecord.ErrorInfo` assignment | ✓ WIRED | `ErrorInfo: errorInfoPtr`（`internal/metricsruntime/usage_plugin.go:200`、`internal/metricspersist/types.go:29`） |
| `sdk/api/handlers/openai/openai_handlers.go` | `sdk/api/handlers/stream_forwarder.go` | `handlers.StreamForwardOptions.WriteTerminalError` | ✓ WIRED | OpenAI streaming path通过 `ForwardStream(..., StreamForwardOptions{WriteTerminalError: ...})` 连接（`sdk/api/handlers/openai/openai_handlers.go:679`） |
| `sdk/api/handlers/gemini/gemini_handlers.go` | `sdk/api/handlers/stream_forwarder.go` | `handlers.StreamForwardOptions.WriteTerminalError` | ✓ WIRED | Gemini streaming path通过 `ForwardStream(..., StreamForwardOptions{WriteTerminalError: ...})` 连接（`sdk/api/handlers/gemini/gemini_handlers.go:321`） |
| `internal/api/handlers/management/metrics.go` | `status_code + error_info` | percentiles: Go 分类；buckets: SQL `CASE` | ✓ WIRED | percentiles 将 `2xx && error_info 非空` 改为 failure（`internal/api/handlers/management/metrics.go:413`）；buckets 将同逻辑下推到 SQL（`internal/api/handlers/management/metrics.go:506`） |

## Requirements Coverage (Phase 5)

| Requirement | Status | Blocking Issue |
|------------|--------|----------------|
| DISP-01（响应结束后显示本次请求指标汇总） | ? NEEDS HUMAN | 本阶段主要修复失败语义/落库/查询切分；“展示端体验”需要通过实际发起 streaming 请求触发 terminal error 来人工确认（见 Human Verification） |
| STOR-02（提供 REST API 查询历史指标） | ✓ SATISFIED | 本阶段不新增 API，但修复其 success/failure 语义，且相关测试通过（`test/metrics_management_test.go:605`） |
| STOR-03（支持百分位统计） | ✓ SATISFIED | percentiles 路径明确把 `status_code=2xx && error_info!=empty` 归类为 failure（`internal/api/handlers/management/metrics.go:413`），并由既有 percentiles 测试数据集覆盖（同文件测试中含 `f2: status=200 errInfo=boom`） |
| STOR-04（支持按时间窗口聚合） | ✓ SATISFIED | buckets path 的 `success_flag` CASE 同样包含 `error_info` 约束并有回归测试锁定（`internal/api/handlers/management/metrics.go:506`、`test/metrics_management_test.go:605`） |

## Test Evidence

- `go test ./...` 通过（2026-01-30 本机执行，含 `sdk/api/handlers/*`、`internal/metricsruntime`、`internal/api/handlers/management`、`test` 包）
- 关键回归用例：
  - `sdk/api/handlers/stream_forwarder_test.go:15`：terminal error → `LastError="boom"`
  - `internal/metricsruntime/usage_plugin_test.go:15`：`LastError` → `MetricRecord.ErrorInfo`（空值时 nil）
  - `sdk/api/handlers/openai/openai_handlers_test.go:15`：无 chunk 时 terminal error 能设置非 2xx status
  - `sdk/api/handlers/gemini/gemini_handlers_test.go:15`：无 chunk 时 terminal error 能设置非 2xx status
  - `test/metrics_management_test.go:605`：buckets mode 下 `200 + error_info` 归类为 failure（failure bucket count=2, success 不变）

## Anti-Patterns Found

无：在 Phase 5 涉及的核心文件中未发现明显的 TODO/FIXME/placeholder/空实现等 stub 信号。

## Human Verification (Recommended)

### 1. Streaming Terminal Error “已输出后失败”路径

**Test:** 发起一个 streaming 请求，让上游在已经输出至少一个 chunk 后返回 terminal error（模拟 upstream 半途失败）。
**Expected:** 客户端侧可能仍看到 HTTP 200（headers 已提交时无法回滚），但数据库 `metrics.error_info` 非空，且 `/v0/management/metrics?mode=percentiles|buckets` 将该记录归入 failure（不污染 success 聚合）。
**Why human:** “headers 是否已提交”取决于真实网络写入/flush 时机，单测只能覆盖“未写入任何 chunk”时的 status 行为。

## Gaps Summary

未发现阻塞性 gaps。Phase 5 的三项 success criteria（写 status、落库失败信号、Query API 分类测试锁定）在代码与测试层面均可验证为闭环。

---

_Verified: 2026-01-30T11:03:15Z_
_Verifier: Claude (gsd-verifier)_
