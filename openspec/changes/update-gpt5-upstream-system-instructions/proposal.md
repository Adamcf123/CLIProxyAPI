## Why
当上游模型为 `gpt-5-*` 或 `gpt-5-codex-*` 时，现有转换层会注入首条 “IGNORE …” 用户消息并写入代理内置 `instructions`。这会改变上游系统提示的优先级与含义，不符合“保持上游语义”的预期，且在多入口（OpenAI Responses、OpenAI Chat Completions、Claude）下行为不一致。

## What Changes
- 当 `model` 名称匹配正则 `^gpt-5(-codex)?-.*$` 时：
  - 禁止注入 “IGNORE ALL YOUR SYSTEM INSTRUCTIONS …” 首条用户消息
  - 禁止注入代理内置 `instructions`
  - 必须将上游提供的系统提示（顶层 `instructions/system` 或 `messages` 中 `role=system` 文本）原样设置为 `instructions`；若不存在则置空
- 非匹配模型：保持现有行为与例外不变（含已存在的 HasPrefix 命中官方指令→提前返回）

## Impact
- 翻译器入口：
  - OpenAI Responses → Codex（request）
  - OpenAI Chat Completions → Codex（request）
  - Claude → Codex（request）
- 测试：为上述三处新增覆盖用例，验证禁注入与使用上游 system 的规则
- 文档：本提案新增规范条款，避免回归

## Risks / Trade-offs
- 当上游 system 与代理内置 `instructions` 存在冲突时，命中 `gpt-5(-codex)?-*` 将以“上游”为准，可能改变既有行为；本提案以明确规则与测试约束规避不确定性。

## Open Questions
- Claude 请求内若同时存在顶层 `system` 与消息内 `role=system`：优先级按“顶层优先，其次合并首个 system 消息文本”执行（见任务中测试约定）。

