---
status: complete
phase: gpt52-openai-response-usage
source: internal/runtime/executor/gpt52_codex_stream_usage_test.go
started: 2026-02-01T21:15:00Z
updated: 2026-02-01T04:59:24Z
---

## Current Test

[testing complete]

## Tests

### 1. gpt-5.2 Responses API (openai-response) 走 /responses 流式
expected: 使用 OpenAI Responses 格式发起 stream=true 时，请求会发到 /responses，Accept=text/event-stream，body 中 model=gpt-5.2
result: pass
notes: |
  通过本地 mock upstream 的集成测试验证：TestGPT52ResponsesStreamRequestAndUsage

### 2. response.completed usage 解析
expected: 解析 response.completed 事件中的 response.usage，提取 input_tokens/output_tokens/total_tokens
result: pass
notes: |
  parseCodexUsage 解析路径已被 TestGPT52CodexUsageParsing 覆盖

### 3. 真实请求：gpt-5.2 response 带 usage 且成功落库
expected: |
  调用本地 /v1/responses (stream=true, model=gpt-5.2) 后：
  - SSE 的 response.completed 事件里 response.usage 不为 null
  - SQLite logs/metrics.db 对应 request_id 行里 input_tokens/output_tokens/total_tokens 均为非 null
result: pass
notes: |
  运行时验证（本机，UTC 2026-02-01 04:56 左右）：
  - 客户端 SSE response.completed usage: input_tokens=2502, output_tokens=6, total_tokens=2508
  - SQLite metrics 行: request_id=42d4df9e7bf82abe provider=codex model=gpt-5.2 streaming=1 status_code=200 input_tokens=2502 output_tokens=6 total_tokens=2508

## Summary

total: 3
passed: 3
issues: 0
pending: 0
skipped: 0

## Gaps

[none]
