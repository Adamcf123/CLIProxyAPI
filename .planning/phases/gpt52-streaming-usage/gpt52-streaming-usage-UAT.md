---
status: complete
phase: gpt52-streaming-usage
source: todo-2026-01-30-support-streaming-usage-tokens-gpt-5-2.md
started: 2026-02-01T20:30:00Z
updated: 2026-02-01T20:45:00Z
---

## Current Test

[testing complete]

## Tests

### 1. GPT-5.2 流式请求获取 usage tokens
expected: 流式响应结束后 stderr 的 metrics_summary 显示 input_tokens/output_tokens 为具体数值（不再是 null），TPS/TPOT 能被计算
result: pass
notes: |
  集成测试验证通过：
  - TestGPT52StreamUsageInjection: 确认 stream_options.include_usage=true 被正确注入请求体
  - 请求体示例: {"model":"gpt-5.2","messages":[],"stream":true,"stream_options":{"include_usage":true}}

### 2. Usage chunk 解析
expected: 上游返回的 usage chunk 能被正确解析，提取 prompt_tokens/completion_tokens/total_tokens
result: pass
notes: |
  TestGPT52StreamUsageParsing 验证了解析逻辑支持标准 OpenAI usage 格式

### 3. 非流式请求不受影响
expected: 非流式请求不应被注入 stream_options
result: pass
notes: |
  TestGPT52NonStreamNotAffected 确认非流式请求保持原样

## Summary

total: 3
passed: 3
issues: 0
pending: 0
skipped: 0

## Gaps

[none]
