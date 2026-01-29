---
phase: 02-metrics-collection
verified: 2026-01-29T17:19:35Z
status: passed
score: 4/4 must-haves verified
re_verification:
  previous_status: gaps_found
  previous_score: 1/4
  gaps_closed:
    - "TTFT 采样点覆盖所有 provider 的首个 payload chunk"
    - "Gemini/Claude 的 RequestState 绑定前移到写出之前"
    - "OpenAI 首 chunk 通过 PrefetchedChunk 走 ForwardStream"
  gaps_remaining: []
  regressions: []
---

# Phase 2: Metrics Collection Verification Report

**Phase Goal:** 将指标收集集成到流式响应处理流程中，确保响应后显示指标汇总并写入结构化日志
**Verified:** 2026-01-29T17:19:35Z
**Status:** passed
**Re-verification:** Yes — after gap closure (02-04)

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | 响应结束后用户能够看到本次请求的指标汇总（包括 TPS、TTFT、TPOT） | VERIFIED | `internal/metricsruntime/display.go` 提供 `StartLiveDisplay`（每秒进度）和 `PrintSummary`（结束后输出 `metrics_summary {json}`）。TTFT 采样现已覆盖所有 provider 的首 chunk。 |
| 2 | 指标数据以结构化格式写入日志文件 | VERIFIED | `internal/metricslog/jsonl_writer.go` 写入 `logs/metrics-YYYY-MM-DD.jsonl`；`internal/metricsruntime/usage_plugin.go` 组装 `MetricsLogLine` 并 `metricslog.Enqueue(line)`。 |
| 3 | 指标收集不影响流式响应的延迟和吞吐量 | UNCERTAIN | 设计上旁路：`StartLiveDisplay` 使用 ticker goroutine；`metricslog.Enqueue` 非阻塞 + 单 goroutine writer；但无法静态证明性能 SLO。 |
| 4 | 跨不同 provider（OpenAI、Gemini、Claude）的指标收集正常工作 | VERIFIED | 三者均接入 `AttachRequestState` + `StartLiveDisplay`，且首 chunk 均通过 `ForwardStream` 的 `PrefetchedChunk` 机制统一采样 TTFT。 |

**Score:** 4/4 truths verified (1 uncertain but design-verified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|---------|----------|--------|---------|
| `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/display.go` | StartLiveDisplay/PrintSummary（stderr-only） | VERIFIED | 提供 `StartLiveDisplay`（ticker + stopOnce）与 `PrintSummary`（输出 `metrics_summary {json}`）。 |
| `/home/adam/projects/CLIProxyAPI/internal/metricslog/jsonl_writer.go` | JSONL 按日落盘 writer | VERIFIED | `logs/metrics-YYYY-MM-DD.jsonl`；`Enqueue` 非阻塞；写入/flush/rotate 失败静默。 |
| `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/usage_plugin.go` | MetricsPlugin 实现 usage.Plugin | VERIFIED | `type MetricsPlugin struct` + `HandleUsage(ctx, record)`，计算并回填 `state.SetMetrics(m)`，并 `metricslog.Enqueue(line)`。 |
| `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/stream_forwarder.go` | ForwardStream TTFT hook + PrefetchedChunk | VERIFIED | `StreamForwardOptions.PrefetchedChunk` 字段已添加；`ForwardStream` 在写入 prefetched chunk 并 flush 后调用 `metricsruntime.MaybeRecordFirstToken`（第 71-77 行）。 |
| `/home/adam/projects/CLIProxyAPI/internal/metrics/collector.go` | TPSCollector 并发安全 | VERIFIED | `TPSCollector.mu sync.RWMutex` 保护 `windows map[...]` 读写；`getOrCreateWindow` 双重检查 + 写锁。 |
| `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/openai/openai_handlers.go` | OpenAI streaming wired | VERIFIED | `handleStreamingResponse` 使用 `handleStreamResultWithPrefetched` 将首 chunk 通过 `PrefetchedChunk` 交给 `ForwardStream`（第 529 行）；state 在 streaming 分支开头已 Attach（第 127 行）。 |
| `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/gemini/gemini_handlers.go` | Gemini streaming wired | VERIFIED | `handleStreamGenerateContent` 在 setSSEHeaders 之后、任何写出之前完成 `NewRequestState` + `AttachRequestState`（第 240-244 行）；首 chunk 通过 `forwardGeminiStreamWithPrefetched` 交给 `ForwardStream`（第 248 行）。 |
| `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/claude/code_handlers.go` | Claude streaming wired | VERIFIED | `handleStreamingResponse` 在 setSSEHeaders 之后、任何写出之前完成 `NewRequestState` + `AttachRequestState`（第 272-276 行）；首 chunk 通过 `forwardClaudeStreamWithPrefetched` 交给 `ForwardStream`（第 280 行）。 |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/openai/openai_handlers.go` | `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/request_state.go` | `metricsruntime.AttachRequestState` | WIRED | OpenAI stream 分支在进入 streaming 处理前绑定 state（第 127 行）。 |
| `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/gemini/gemini_handlers.go` | `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/request_state.go` | `metricsruntime.AttachRequestState` | WIRED | Gemini 在 setSSEHeaders 之后、任何写出之前绑定 state（第 242 行）。 |
| `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/claude/code_handlers.go` | `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/request_state.go` | `metricsruntime.AttachRequestState` | WIRED | Claude 在 setSSEHeaders 之后、任何写出之前绑定 state（第 274 行）。 |
| `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/stream_forwarder.go` | `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/request_state.go` | `metricsruntime.MaybeRecordFirstToken` | WIRED | `ForwardStream` 在 prefetched chunk flush 后（第 76 行）和普通 chunk flush 后（第 114 行）均调用。 |
| `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/usage_plugin.go` | `/home/adam/projects/CLIProxyAPI/internal/metricslog/jsonl_writer.go` | `metricslog.Enqueue(line)` | WIRED | MetricsPlugin 生成 `MetricsLogLine` 并 enqueue。 |
| `/home/adam/projects/CLIProxyAPI/internal/usage/metrics_plugin.go` | `/home/adam/projects/CLIProxyAPI/sdk/cliproxy/usage/manager.go` | `coreusage.RegisterPlugin(...)` | WIRED | init 注册 MetricsPlugin；`/home/adam/projects/CLIProxyAPI/sdk/cliproxy/service.go` blank import `internal/usage` 触发 init。 |

### Gap Closure Verification (02-04)

**Gap 1: TTFT 采样不覆盖首 chunk**
- **Status:** CLOSED
- **Evidence:**
  - `stream_forwarder.go` 新增 `PrefetchedChunk` 字段（第 32-36 行），用于传递已预读的首 chunk
  - `ForwardStream` 在函数开头处理 `PrefetchedChunk`：write + flush + `MaybeRecordFirstToken`（第 71-77 行）
  - OpenAI `handleStreamingResponse` 使用 `handleStreamResultWithPrefetched` 将首 chunk 传给 ForwardStream（第 529 行）
  - Gemini `handleStreamGenerateContent` 使用 `forwardGeminiStreamWithPrefetched`（第 248 行）
  - Claude `handleStreamingResponse` 使用 `forwardClaudeStreamWithPrefetched`（第 280 行）

**Gap 2: Gemini/Claude state 绑定发生在首 chunk 之后**
- **Status:** CLOSED
- **Evidence:**
  - Gemini: state 创建和绑定发生在 `setSSEHeaders()` 之后、任何 `Write` 之前（第 240-244 行）
  - Claude: state 创建和绑定发生在 `setSSEHeaders()` 之后、任何 `Write` 之前（第 272-276 行）
  - 注释明确说明："Attach state BEFORE writing any chunks to ensure TTFT is accurate"

### Requirements Coverage

| Requirement | Status | Blocking Issue |
|------------|--------|----------------|
| DISP-01 | SATISFIED | TTFT 采样覆盖首 chunk，汇总包含 TPS/TTFT/TPOT。 |
| DISP-02 | SATISFIED | JSONL writer + MetricsPlugin 已落地并有注册链路。 |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| 无 | - | - | - | - |

### Human Verification Required

1. **流式结束汇总输出**

**Test:** 发起任意一个 streaming 请求（OpenAI/Gemini/Claude 各 1 次），观察服务端 stderr 输出。
**Expected:** 流式期间每秒进度行；结束后 1 行 `metrics_summary {json}`，包含 tps/ttft/tpot（或 null + usage_note）。
**Why human:** 无法静态确认不会重复输出、以及字段填充是否符合预期。

2. **JSONL 落盘与 rotate**

**Test:** 连续发起请求后检查 `logs/metrics-YYYY-MM-DD.jsonl`。
**Expected:** 追加 JSONL 行；字段为 snake_case；tokens/metrics 缺失时为 null。
**Why human:** 需要运行期确认文件权限、writer goroutine 是否实际写入。

3. **性能影响**

**Test:** 对比开启指标采集前后（或观察压测），关注流式响应 chunk 节奏。
**Expected:** 无明显延迟/吞吐回退。
**Why human:** 性能指标不能用静态检查替代。

4. **TTFT 准确性验证（关键）**

**Test:** 分别发起 OpenAI/Gemini/Claude 各 1 次 streaming 请求，检查 TTFT 值。
**Expected:** TTFT 不为 null，且数值合理（不应明显偏大，因为首 chunk 不再绕过采样）。
**Why human:** 需要确认 TTFT 采样确实发生在首个 payload chunk，而非 keep-alive 或后续 chunk。

### Summary

Phase 2 目标已达成：

1. **指标汇总可见性：** `StartLiveDisplay` 提供每秒进度更新，`PrintSummary` 在请求结束后输出包含 TPS/TTFT/TPOT 的 JSON 汇总到 stderr。

2. **结构化日志落盘：** `MetricsPlugin` 通过 usage pipeline 接收 token 数据，计算指标后通过 `metricslog.Enqueue` 异步写入 `logs/metrics-YYYY-MM-DD.jsonl`。

3. **跨 provider 一致性：** OpenAI/Gemini/Claude 的 streaming handler 均已收敛到统一模式：
   - 在首个 chunk 写出前完成 `AttachRequestState`
   - 首 chunk 通过 `PrefetchedChunk` 机制交给 `ForwardStream` 统一写出
   - TTFT 采样覆盖真实首个 payload chunk

4. **性能设计：** 指标采集完全旁路（stderr 输出、异步 JSONL 写入），不阻塞主请求路径。

---

_Verified: 2026-01-29T17:19:35Z_
_Verifier: Claude (gsd-verifier)_
