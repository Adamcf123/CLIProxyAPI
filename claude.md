# MiniMax 模型说明

本项目中的 MiniMax 模型（如 MiniMax-M2.7-highspeed）使用 Anthropic API 格式（Claude executor），
通过 `/v1/messages` 端点处理，thinking 配置采用 Claude 格式（`thinking.type` + `budget_tokens`）。
不要将 MiniMax 模型路由到 IFlowExecutor；IFlow 在本项目中不使用。
