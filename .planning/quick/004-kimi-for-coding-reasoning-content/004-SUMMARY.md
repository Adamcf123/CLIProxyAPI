---
phase: quick-004-kimi-for-coding-reasoning-content
plan: 004
subsystem: api
tags: [kimi-for-coding, claude, thinking, messages]

provides:
  - "保留 thinking 的同时，为 kimi-for-coding 上游 /v1/messages 补齐 tool-call assistant message 的 reasoning_content，避免 400"

key_files:
  modified:
    - internal/runtime/executor/claude_executor.go
    - internal/runtime/executor/claude_executor_test.go

completed: 2026-02-01
---

# Quick Task 004: kimi-for-coding Summary

**在保持 thinking.enabled 的前提下，代理侧为历史中的 assistant tool_use 消息补齐 reasoning_content（允许为空字符串），从而满足上游校验并避免 400。**

## What Changed

- 归一化逻辑（`internal/runtime/executor/claude_executor.go`）新增 `ensureKimiToolCallReasoningContent`：
  - 仅对 `model=kimi-for-coding` 且 `thinking.type==enabled` 生效
  - 遍历 `messages[]`，对缺失 `reasoning_content` 的 assistant tool_use 消息写入空字符串
- 覆盖非流式 + 流式（`ClaudeExecutor.Execute` / `ClaudeExecutor.ExecuteStream`）均在转发前调用该归一化

## Why

Kimi/网关在 thinking 开启时会要求：任何 assistant tool call message 必须携带 `reasoning_content` 字段（即使为空）。
OpenCode 在“直接 tool-call 且没有显式 reasoning 段”时不会生成该字段，导致历史一旦出现缺字段消息，后续请求持续 400。

## Tests

- `go test ./internal/runtime/executor -run TestEnsureKimiToolCallReasoningContent`

## Task Commits

- `283d682` fix(quick-004-kimi-for-coding-004): add reasoning_content for tool_call messages
- `3af2a38` test(quick-004-kimi-for-coding-004): cover reasoning_content backfill
