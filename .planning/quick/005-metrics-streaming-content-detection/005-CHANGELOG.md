# 005 - Metrics Streaming Content Detection (Changelog)

Date: 2026-02-02

## 业务改动说明

本次修改聚焦于一个用户可感知的指标质量问题：在 streaming 响应里，系统经常无法稳定识别“首个用户可见内容 token”，导致 `ttft/tps/tpot` 频繁输出为 `null`。

修复后，OpenAI Responses 风格的 SSE（`event:` 行承载事件类型、`data:` 行仅包含 `{"delta":"..."}` 的 payload）也能稳定命中“首个内容 chunk”，从而让 `ttft` 有值；`tps/tpot` 仍然遵循可信度门槛（例如 output_tokens 太少或内容 chunk 太少时继续输出 `null`），避免速率被极短窗口误导。

同时按“方案 B”的口径：对工具/思考型模型（如 kimi-for-coding），将 thinking/tool 相关的 delta 也视为“内容”（可触发 TTFT），避免出现 output_tokens 很大但 TTFT 仍为 null 的情况。

## 技术改动说明

### 1) 统一/增强 streaming 内容识别（TTFT 触发）

做了什么（`internal/metricsruntime/request_state.go`）：

- 将 streaming 内容识别升级为“按 SSE 行（event/data）增量解析，并在 RequestState 内做小 buffer 拼行”，避免依赖 `\n\n` 帧分隔符才能命中 TTFT。
- 兼容“chunk 不带换行”的写法：当执行器/translator 输出的是单行 `data: ...`（handler 另外再写 `"\n"`）时，也能直接解析并命中 TTFT。
- 支持 Responses SSE 的 event/data 拆分：当 `event: response.output_text.delta` 而 `data` 里没有 `type` 字段时，仍能通过 eventName + payload（`delta/text/part.text` 等）判断是否为用户可见内容。
- 支持 TCP/HTTP 分片：当一个 SSE 行被拆到多个 write 中（例如 JSON 被切开），通过 request 级缓冲等待行结束（`\n`）后再解析。
- 对“上游/本服务输出只逐行 flush、没有稳定空行分隔”的情况更鲁棒：只要 `data:` 行完整到达，就能识别首个内容 chunk。
- 扩展了 Responses JSON 形态识别：补齐 `response.content_part.added/done`（`part.type=output_text`）以及 `response.output_item.added/done`（message item 的 content blocks）作为“可见内容”的判断来源。

相关测试（`internal/metricsruntime/request_state_test.go`）：

- 新增用例覆盖 `event: response.output_text.delta` + `data: {"delta":"hi"}` 的 TTFT 命中。
- 新增用例覆盖同一 SSE 帧拆分到两次写入（partial frame -> complete frame）仍能命中 TTFT。

### 2) 修复 Responses SSE 输出帧的结束分隔符

做了什么：修复多个 Responses 相关 translator 生成的 SSE 事件未以空行结尾的问题（缺少 `\n\n`）。缺少帧分隔会导致“按帧解析”的逻辑无法闭合帧，从而让 TTFT 永远不触发。

涉及文件：

- `internal/translator/openai/openai/responses/openai_openai-responses_response.go`
- `internal/translator/claude/openai/responses/claude_openai-responses_response.go`
- `internal/translator/gemini/openai/responses/gemini_openai-responses_response.go`

修改点：`emit*Event(...)` 统一输出 `event: ...\ndata: ...\n\n`（标准 SSE 帧，空行结束）。

## 验证

本地运行：

- `gofmt -w ...`
- `go test ./...`

## 影响面/注意事项

- TTFT 识别更“早”和更“稳”：对 Responses SSE 事件拆分与分片情况不再漏判。
- `tps/tpot` 仍受可信度门槛控制：当 output_tokens 太少或有效内容 chunk 太少时，继续返回 `null`（这是刻意行为，不视为缺陷）。
- RequestState 内新增 SSE 小缓冲（上限 64KiB），用于跨 chunk 拼帧；该缓冲仅在 streaming + 疑似 SSE 时启用。
