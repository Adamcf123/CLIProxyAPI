# Task 003 (Green): parseClaudeUsage 提取 ephemeral cache tokens — 实现

**depends-on**: task-003-claude-ephemeral-cache-test.md
**type**: impl (Green)
**paired-test**: task-003-claude-ephemeral-cache-test.md

## Goal

修改 `internal/runtime/executor/usage_helpers.go` 中的 `parseClaudeUsage` 和 `parseClaudeStreamUsage`，添加 ephemeral cache tokens 提取和 `thinking_tokens` 提取，使 task-003 的测试通过。

## BDD Scenario

```gherkin
Scenario: parseClaudeUsage 合并 ephemeral cache tokens
  Given 响应包含 usage.cache_creation.ephemeral_5m_input_tokens=100，ephemeral_1h_input_tokens=50
  When  调用 parseClaudeUsage
  Then  CachedTokens == 150

Scenario: parseClaudeUsage 提取 thinking_tokens
  Given 响应包含 usage.thinking_tokens=200，output_tokens=30
  When  调用 parseClaudeUsage
  Then  ReasoningTokens == 200，TotalTokens == 230

Scenario: 回归：ephemeral 为 0 时保持原有 cache_read 逻辑
  Given 响应包含 usage.cache_read_input_tokens=500，ephemeral 字段均为 0
  When  调用 parseClaudeUsage
  Then  CachedTokens == 500
```

## Files to Modify

- **Modify**: `internal/runtime/executor/usage_helpers.go`

### 修改 `parseClaudeUsage`（约第 271-287 行）

在现有 `CachedTokens` 赋值之后、`TotalTokens` 计算之前，添加：
1. 读取 `usageNode.Get("cache_creation.ephemeral_5m_input_tokens").Int()` 和 `ephemeral_1h_input_tokens`，累加到 `detail.CachedTokens`
2. 读取 `usageNode.Get("thinking_tokens").Int()`，赋值到 `detail.ReasoningTokens`
3. 更新 `TotalTokens` 为 `InputTokens + OutputTokens + ReasoningTokens`

### 修改 `parseClaudeStreamUsage`（约第 289-308 行）

与 `parseClaudeUsage` 相同的改动：
1. 添加 ephemeral cache tokens 提取（累加到 `CachedTokens`）
2. 添加 `thinking_tokens` 提取（赋值到 `ReasoningTokens`）
3. 更新 `TotalTokens`

### 原有 CachedTokens 逻辑保留

原有的 `if detail.CachedTokens == 0 { detail.CachedTokens = usageNode.Get("cache_creation_input_tokens").Int() }` 逻辑在 ephemeral 累加之后执行——需确保：
- 先累加 ephemeral
- 然后判断 `CachedTokens == 0` 决定是否回退到 `cache_creation_input_tokens`

## Steps

1. 读取 `internal/runtime/executor/usage_helpers.go`，定位 `parseClaudeUsage` 和 `parseClaudeStreamUsage` 函数
2. 在两个函数中添加 ephemeral cache 提取（`cache_creation.ephemeral_5m_input_tokens` + `ephemeral_1h_input_tokens`）
3. 添加 `thinking_tokens` → `ReasoningTokens` 提取
4. 更新 `TotalTokens = InputTokens + OutputTokens + ReasoningTokens`
5. 确保原有 `cache_creation_input_tokens` 回退逻辑位于 ephemeral 累加之后
6. 运行所有 usage_helpers 测试

## Verification

```bash
go test ./internal/runtime/executor/... -run TestParseClaudeUsage -v
go test ./internal/runtime/executor/... -run TestParseClaudeStreamUsage -v
```

期望结果：task-003 测试 **通过**（Green）。

```bash
go test ./internal/runtime/executor/...
```

期望结果：所有 executor 测试通过（无回归）。

```bash
go build ./...
```

期望结果：编译无错误。
