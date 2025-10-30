# Tasks for change filter-bashoutput-20251030

1. 请求侧过滤：在 `ConvertClaudeRequestToCodex` 中跳过 BashOutput 的 function_call（已实现）
2. 流式响应过滤：在 `ConvertCodexResponseToClaude` 中基于 output_index 跳过 BashOutput（已实现）
3. 非流式响应过滤：在 `ConvertCodexResponseToClaudeNonStream` 中跳过 BashOutput（已实现）
4. stop_reason 一致性：仅在存在有效工具调用时输出 `tool_use`（已实现）
5. 单测：新增流式与非流式用例覆盖 BashOutput 过滤（已实现）
6. 文档：记录判定规则与风险（进行中）
