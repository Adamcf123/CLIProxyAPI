# Task 003 (Red): parseClaudeUsage 提取 ephemeral cache tokens — 测试

**depends-on**: —
**type**: test (Red)
**paired-impl**: task-003-claude-ephemeral-cache-impl.md

## Goal

在 `internal/runtime/executor/usage_helpers_test.go` 中添加失败的单元测试，验证 `parseClaudeUsage` 和 `parseClaudeStreamUsage` 正确处理：
1. `cache_creation.ephemeral_5m_input_tokens` 和 `ephemeral_1h_input_tokens`（合并到 CachedTokens）
2. `thinking_tokens`（填充到 ReasoningTokens）
3. `TotalTokens` 包含 ReasoningTokens

## Background

真实 OAuth 抓包（`docs/claude-cli-2.1.62-request-oauth登录模式.txt`）中，Claude API 的 `message_start` 事件包含：
```json
"cache_creation": {
  "ephemeral_5m_input_tokens": 100,
  "ephemeral_1h_input_tokens": 50
}
```
当前 `parseClaudeUsage` 只提取 `cache_read_input_tokens` 和 `cache_creation_input_tokens`，忽略了这两个 ephemeral 字段。

## BDD Scenario

```gherkin
Scenario: parseClaudeUsage 合并 ephemeral cache tokens 到 CachedTokens
  Given 一个 Claude 非流式响应 JSON，包含：
        usage.cache_creation.ephemeral_5m_input_tokens=100，
        usage.cache_creation.ephemeral_1h_input_tokens=50，
        usage.cache_read_input_tokens=0，
        usage.input_tokens=10，usage.output_tokens=20
  When  调用 parseClaudeUsage(data)
  Then  detail.CachedTokens == 150（100 + 50）
  And   detail.InputTokens == 10
  And   detail.OutputTokens == 20
  And   detail.TotalTokens == 30（input + output，不含 cache）

Scenario: parseClaudeStreamUsage 合并 ephemeral cache tokens 到 CachedTokens
  Given 一个 SSE 行：data: {..., "usage": {"cache_creation": {"ephemeral_5m_input_tokens": 80, "ephemeral_1h_input_tokens": 20}, "output_tokens": 15}}
  When  调用 parseClaudeStreamUsage(line)
  Then  ok == true
  And   detail.CachedTokens == 100（80 + 20）
  And   detail.OutputTokens == 15

Scenario: parseClaudeUsage 提取 thinking_tokens 到 ReasoningTokens
  Given 一个 Claude 非流式响应 JSON，包含 usage.thinking_tokens=200，usage.output_tokens=30
  When  调用 parseClaudeUsage(data)
  Then  detail.ReasoningTokens == 200
  And   detail.TotalTokens == 230（output + reasoning）

Scenario: ephemeral 字段为 0 时行为不变（回归）
  Given 一个 Claude 响应，usage.cache_read_input_tokens=500，cache_creation.ephemeral_5m_input_tokens=0
  When  调用 parseClaudeUsage(data)
  Then  detail.CachedTokens == 500（沿用 cache_read）
```

## Files to Create / Modify

- **Modify**: `internal/runtime/executor/usage_helpers_test.go`
  - 新增 `TestParseClaudeUsage_EphemeralCacheTokens`
  - 新增 `TestParseClaudeStreamUsage_EphemeralCacheTokens`
  - 新增 `TestParseClaudeUsage_ThinkingTokens`
  - 新增 `TestParseClaudeUsage_EphemeralZeroRegression`

## Steps

1. 找到 `internal/runtime/executor/usage_helpers_test.go`，了解现有 Claude 解析测试的写法
2. 构造包含 `cache_creation.ephemeral_5m_input_tokens` 的 JSON 字节切片
3. 调用 `parseClaudeUsage` 并断言 `CachedTokens == ephemeral_5m + ephemeral_1h`
4. 对流式版本构造 SSE 行（`data: {...}`格式）并测试 `parseClaudeStreamUsage`
5. 构造含 `thinking_tokens` 的 JSON 并验证 ReasoningTokens
6. 运行测试确认红色

## Verification

```bash
go test ./internal/runtime/executor/... -run TestParseClaudeUsage -v
go test ./internal/runtime/executor/... -run TestParseClaudeStreamUsage -v
```

期望结果：新增测试 **失败**（Red），现有测试仍通过。
