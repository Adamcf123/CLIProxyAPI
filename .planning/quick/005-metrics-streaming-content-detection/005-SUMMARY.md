---
phase: quick-005-metrics-streaming-content-detection
plan: 005
subsystem: metrics
tags: [metrics, streaming, sse, tps, ttft, tpot, openai-responses, kimi-for-coding, claude]

provides:
  - "OpenAI Responses/Codex streaming supports content detection for response.output_text.delta so TTFT/TPS/TPOT are not mostly null"
  - "Claude/Kimi tool streaming treats input_json_delta partial_json as content so tool-heavy streams can produce TTFT/TPS/TPOT"
  - "SSE chunk parsing counts multiple data: events inside one chunk for ContentTokenChunks confidence gating"

key_files:
  modified:
    - internal/metricsruntime/request_state.go
    - internal/metricsruntime/usage_plugin.go
    - internal/metricsruntime/request_state_test.go

completed: 2026-02-02
---

# Quick Task 005: Streaming Metrics Non-Null Fixes

**修复 streaming 指标大量为 null：补齐 OpenAI Responses/Codex 内容事件识别，并让 kimi-for-coding 的 tool output（input_json_delta）也能触发 TTFT/TPS/TPOT。**

## What Changed

- 内容 token 识别（`internal/metricsruntime/request_state.go`）新增支持：
  - OpenAI Responses: `type=response.output_text.delta` + 非空 `delta` 视为内容 token
  - Claude tool streaming: `type=content_block_delta` + `delta.type=input_json_delta` + 非空 `delta.partial_json` 视为内容 token
- SSE 计数方式升级：同一个 chunk 内的多个 `data:` event 会逐个计数，避免因为上游/代理合并写导致 `ContentTokenChunks` 永远达不到阈值
- streaming 的 post-first-token 最小持续时间门槛从 300ms 调整到 50ms，降低短响应被错误抑制为 null 的比例

## Why

- 运行时 metrics.db 中 `tps/ttft/tpot` 大量为 null 的主要原因并非缺失 usage，而是首个内容 token 无法被识别（尤其是 OpenAI Responses 的 event 形态，以及 kimi-for-coding 的 tool args 增量输出）。

## Tests

- `go test ./...`

## Task Commits

- `233f709` fix(metrics): reduce streaming TTFT/TPS nulls

## Notes

- 该修复只影响新写入的 metrics 行；历史已落库数据不会自动回填重算。
