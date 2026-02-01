---
status: complete
phase: gpt52-openai-response-usage
source: internal/runtime/executor/gpt52_codex_stream_usage_test.go
started: 2026-02-01T21:15:00Z
updated: 2026-02-01T21:15:00Z
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

## Summary

total: 2
passed: 2
issues: 0
pending: 0
skipped: 0

## Gaps

[none]
