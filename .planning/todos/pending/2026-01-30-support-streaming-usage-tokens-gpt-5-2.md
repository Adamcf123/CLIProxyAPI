---
created: 2026-01-30T14:30
title: Support streaming usage tokens for gpt-5.2
area: general
files:
  - internal/runtime/executor/openai_compat_executor.go:169
  - internal/runtime/executor/usage_helpers.go:216
  - internal/metricsruntime/usage_plugin.go
  - sdk/api/handlers/openai/openai_handlers.go
---

{{LANGUAGE_DIRECTIVE}}

## Problem

当前使用 `model=gpt-5.2` 走流式（`/v1/chat/completions` + `stream=true`）时，经常拿不到 usage/tokens（`input_tokens/output_tokens=null`，`usage_note=usage_missing_tokens_unavailable`），导致：

- JSONL 指标里缺少 tokens，无法稳定计算 TPS/TPOT
- stderr 的 `metrics_summary` 只能输出 TTFT（我们已改为“首个内容 token”口径），但吞吐相关字段会是 null

我们讨论过的可行方向是：如果上游支持 OpenAI 的 `stream_options.include_usage=true`，由客户端在请求里带上该字段，服务端需要确保该字段能透传到上游，并且在流式过程中解析到 usage chunk（`parseOpenAIStreamUsage`）后发布 usage record。

## Solution

TBD（优先做“透传 + 验证”，避免代理侧做 tokenizer 估算）：

1) 确认 `gpt-5.2` 在当前配置下实际命中的 provider/executor（为什么 summary 里 provider=openai）。
2) 确保对该路径：
   - 请求 JSON 的 `stream_options.include_usage` 不会被 translator/重写逻辑丢弃
   - 上游若返回 usage chunk，`internal/runtime/executor/usage_helpers.go:parseOpenAIStreamUsage` 能解析并 publish
3) 增补回归：
   - 同一模型流式请求，对比带/不带 `stream_options.include_usage=true` 的 tokens 差异
   - 确认 metrics JSONL 与 stderr summary 的 tokens/TPS/TPOT 行为符合预期
