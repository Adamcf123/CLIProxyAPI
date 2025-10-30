## MODIFIED Requirements

### Requirement: Provider Model Registration
系统 SHALL 基于认证条目的实际后端特征注册可用模型列表，以保证 `/v1/models` 的可用性与准确性。

#### Scenario: GPT‑5 family uses upstream system instructions
- **GIVEN** 请求 `model` 名称匹配 `^gpt-5(-codex)?-.*$`
- **WHEN** 系统在任一转换入口（OpenAI Responses / OpenAI Chat Completions / Claude）将上游请求转换为 Codex 请求
- **THEN** 系统 SHALL 不注入首条 “IGNORE …” 用户消息
- **AND** 系统 SHALL 不注入代理内置 `instructions`
- **AND** 系统 SHALL 将上游系统提示（顶层 `instructions/system` 或 `messages` 中首个 `role=system` 文本）原样设置为 Codex 的 `instructions` 字段
- **AND** 若上游不存在系统提示，Codex 的 `instructions` SHALL 置为空字符串

#### Scenario: Non‑GPT‑5 models keep current behavior
- **GIVEN** 请求 `model` 名称不匹配 `^gpt-5(-codex)?-.*$`
- **THEN** 系统 SHALL 维持现有注入与例外逻辑（包括已存在的 HasPrefix 官方指令→提前返回）
