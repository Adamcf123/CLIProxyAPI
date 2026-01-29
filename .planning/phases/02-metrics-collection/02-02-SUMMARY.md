---
phase: 02-metrics-collection
plan: "02"
subsystem: infra
tags: [go, gin, streaming, metrics, stderr, tty, tps, ttft, tpot]

# Dependency graph
requires:
  - phase: 02-metrics-collection
    provides: RequestState + ForwardStream TTFT hook (02-01)
provides:
  - Server-side live metrics progress rendering (stderr-only, TTY-aware)
  - Per-request summary line output for streaming requests (metrics_summary JSON)
affects: [02-03, observability, logs, streaming]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Side-channel stderr progress ticker (no response body mutation)
    - TTY-aware single-line overwrite vs newline logging
    - RequestState snapshot API for safe concurrent reads

key-files:
  created:
    - internal/metricsruntime/display.go
  modified:
    - internal/metricsruntime/request_state.go
    - sdk/api/handlers/openai/openai_handlers.go
    - sdk/api/handlers/openai/openai_responses_handlers.go
    - sdk/api/handlers/claude/code_handlers.go
    - sdk/api/handlers/gemini/gemini_handlers.go
    - sdk/api/handlers/gemini/gemini-cli_handlers.go

key-decisions:
  - "Live display and summaries write only to os.Stderr (never into HTTP response bodies)"
  - "TTY detection gates line-overwrite (\\r + ANSI clear); non-TTY falls back to newline output"
  - "Summary is emitted as a single searchable line: metrics_summary <json>; missing usage keeps tokens/throughput as null"

patterns-established:
  - "Pattern: StartLiveDisplay(state) returns an idempotent stop() that prints summary once"

# Metrics
duration: 19min
completed: 2026-01-29
---

# Phase 02 Plan 02: Metrics Live Display Summary

**在服务端为流式请求提供每秒更新的指标进度行与结束汇总行（stderr-only、TTY-aware、不污染 SSE/JSON 响应体）。**

## Performance

- **Duration:** 19 min
- **Started:** 2026-01-29T16:29:29Z
- **Completed:** 2026-01-29T16:48:20Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments

- 流式响应期间（server 侧）每秒在 stderr 输出同一行进度（TTY 下同行覆盖；非 TTY 下换行）
- 流式结束后输出一次可检索的汇总单行：`metrics_summary {json}`（TPS/TTFT/TPOT/token/耗时/status 等；缺失则为 null 并标注原因）
- 各 provider streaming handler 在进入 ForwardStream 前统一 AttachRequestState + 启动/停止显示器

## Task Commits

Each task was committed atomically:

1. **Task 1: 实现 server 侧实时显示与结束汇总输出** - `8588aad` (feat)
2. **Task 2: 在各 provider streaming handler 中创建 request state 并触发显示/汇总** - `c768a82` (feat)

## Files Created/Modified

- `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/display.go` - stderr-only 实时进度渲染 + 汇总输出（TTY-aware）
- `/home/adam/projects/CLIProxyAPI/internal/metricsruntime/request_state.go` - 扩展 RequestState（tokens/status/path/metrics）并提供 Snapshot/Setter API
- `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/openai/openai_handlers.go` - OpenAI streaming 路径创建 state 并启动显示
- `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/openai/openai_responses_handlers.go` - OpenAI Responses streaming 路径创建 state 并启动显示
- `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/claude/code_handlers.go` - Claude streaming 路径创建 state 并启动显示
- `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/gemini/gemini_handlers.go` - Gemini streaming 路径创建 state 并启动显示
- `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/gemini/gemini-cli_handlers.go` - Gemini CLI streaming 路径创建 state 并启动显示

## Decisions Made

- 采用“旁路显示器”模型：进度渲染与汇总输出只写 `os.Stderr`，避免任何对 SSE/JSON payload 的侵入。
- 采用 TTY-aware 行渲染：仅当 stderr 是 TTY 才使用 `\\r` + ANSI 清行，否则换行输出以避免污染非交互日志。
- 汇总输出固定为单行 `metrics_summary <json>`，并在 usage 缺失时保持 `input_tokens/output_tokens/tps/tpot` 为 `null`（不猜测 tokens）。

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 02-02 已把“用户可见输出层”就位；02-03 只需在 usage 插件侧把 tokens/metrics 回填到 RequestState，即可让汇总行自动包含 TPS/TTFT/TPOT 与 tokens。

---
*Phase: 02-metrics-collection*
*Completed: 2026-01-29*
