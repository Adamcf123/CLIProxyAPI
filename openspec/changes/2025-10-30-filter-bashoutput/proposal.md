# Change Proposal: Filter invalid BashOutput tool calls at server side

- change-id: filter-bashoutput-20251030
- status: draft
- owner: runtime-exec
- created: 2025-10-30

## Summary
从 CliproxyAPI 服务器侧拦截并过滤无效的 BashOutput 工具调用，防止错误调用传递到 Claude Code 端，不依赖客户端 `run_in_background=true`。覆盖请求转换与响应转换（流式/非流式）两个方向，同时确保 `stop_reason` 行为保持一致性。

## Motivation
- 现状：Codex 会话在未后台运行情况下，紧接调用 BashOutput，`bash_id` 实为补全ID（如 `chatcmpl-*`），导致 Claude Code 侧反复报 `No shell found`。
- 目标：在服务端识别并屏蔽无效 BashOutput 调用，避免错误外泄、降低用户困扰，并保持其他工具调用不受影响。

## Scope
- 请求侧（Claude → Codex）：在 tool_use → function_call 转换阶段，直接跳过 BashOutput。
- 响应侧（Codex → Claude）：
  - 流式：跳过 BashOutput 的 `response.output_item.*` 与对应的 `arguments.delta`；不产生 `tool_use` 事件。
  - 非流式：跳过 BashOutput 的 `function_call` 项，不写入 content。
- 行为一致性：过滤后 `stop_reason` 不再因为该工具被标记为 `tool_use`，而按实际存在的有效工具决定，否则为 `end_turn`。

## Non-Goals
- 不修改上游模型行为；不引入服务端真正的 shell 执行或 session 管理。
- 不变更其他工具的交互语义。

## Risks
- 误判：工具名变体或非标准参数格式。通过不区分大小写包含匹配与 JSON 校验降低风险。
- 行为回归：通过最小化修改和定向单测控制。

## Validation
- 单测覆盖：
  - 流式：不产生 BashOutput 的 tool_use 事件，最终 `stop_reason` 为 `end_turn`。
  - 非流式：最终 content 不含 BashOutput 的 `tool_use` block。
- 指标（后续可选）：统计被过滤的 BashOutput 次数，用于运维观察。
