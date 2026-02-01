
phase: quick-002-kimi-for-coding
plan: 002
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/runtime/executor/claude_executor.go
  - internal/runtime/executor/claude_executor_test.go
autonomous: true

must_haves:
  truths:
    - "当客户端请求 /v1/messages 且 model=kimi-for-coding 时，即使上游请求体携带 thinking.enabled，代理也会在转发前自动禁用 thinking，从而避免 Kimi Coding API 因缺失 reasoning_content 返回 400"
    - "该修复同时覆盖非流式与流式（Execute / ExecuteStream），且不改变其他模型的既有行为（thinking 仍按现有规则处理）"
    - "不引入任何新开关/参数；行为仅由 model==kimi-for-coding 决定，且可通过单元测试锁定"
  artifacts:
    - path: "internal/runtime/executor/claude_executor.go"
      provides: "Claude 上游请求体的 model 兼容性归一化（kimi-for-coding 禁用 thinking）"
    - path: "internal/runtime/executor/claude_executor_test.go"
      provides: "kimi-for-coding 禁用 thinking 的回归测试"
  key_links:
    - from: "internal/runtime/executor/claude_executor.go"
      to: "internal/thinking/strip.go"
      via: "thinking.StripThinkingConfig(provider=claude)"
      pattern: "StripThinkingConfig\(.*\"claude\"\)"
    - from: "internal/runtime/executor/claude_executor.go"
      to: "ClaudeExecutor.Execute / ExecuteStream"
      via: "在转发前对 body 做一次 kimi-for-coding 兼容性归一化"
      pattern: "Execute(Stream)?\(.*\)"
---

<objective>
在代理层面为 `kimi-for-coding` 增加上游兼容性处理：当该模型被选择时，自动禁用 Claude Messages 请求体里的 `thinking` 字段，避免 Kimi Coding API 因“thinking enabled 但 tool call 缺 reasoning_content”返回 400，从而让 opencode/Claude Code 工具链可稳定使用该模型。

Purpose: 以最小、可验证、无新开关的方式修复 kimi-for-coding 的请求不兼容问题。
Output: Claude executor 的请求归一化逻辑 + 回归测试。
</objective>

<execution_context>
@~/.config/opencode/get-shit-done/workflows/execute-plan.md
@~/.config/opencode/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@internal/runtime/executor/claude_executor.go
@internal/runtime/executor/claude_executor_test.go
@internal/thinking/strip.go
@logs/error-v1-messages-2026-02-01T140601-cc2110c08757e092.log
</context>

<tasks>

<task type="auto">
  <name>Task 1: kimi-for-coding 转发前强制禁用 thinking（非流式 + 流式）</name>
  <files>internal/runtime/executor/claude_executor.go</files>
  <action>
  在 Claude 执行器（`internal/runtime/executor/claude_executor.go`）里增加一个明确、可复用的“上游兼容性归一化”步骤，并在非流式与流式都调用：
  - 新增一个小的私有 helper（例如 `normalizeClaudeUpstreamPayload(baseModel string, body []byte) []byte`）：
    - 仅当 `baseModel == "kimi-for-coding"` 时，删除请求体里的 `thinking` 字段（使用 `thinking.StripThinkingConfig(body, "claude")` 或等价的 `sjson.DeleteBytes(body, "thinking")`）。
    - 其余模型原样返回，不引入任何额外参数/配置。
  - 在 `ClaudeExecutor.Execute` 与 `ClaudeExecutor.ExecuteStream` 中，在“转发到上游前的最终 body”上调用该 helper：
    - 建议位置：`disableThinkingIfToolChoiceForced(body)` 之后、`extractAndRemoveBetas(body)` 之前（确保后续流程不会重新依赖/注入 thinking）。
  - 保持现有逻辑不变：cloaking、payload config、cache_control、tool prefix、betas header 等都继续按既有规则执行，只额外对 kimi-for-coding 做 thinking 字段剥离。
  </action>
  <verify>
  go test ./... 
  </verify>
  <done>
  - 当请求 model=kimi-for-coding 且 body 含 `thinking` 时，转发到上游的 payload 不再包含 `thinking`
  - 该修改对 Execute 与 ExecuteStream 都生效
  - 其他模型的 `thinking` 字段不被误删
  </done>
</task>

<task type="auto">
  <name>Task 2: 添加 kimi-for-coding 禁用 thinking 的回归测试</name>
  <files>internal/runtime/executor/claude_executor_test.go</files>
  <action>
  在 `internal/runtime/executor/claude_executor_test.go` 增加单元测试，锁定“kimi-for-coding 必须剥离 thinking”的契约：
  - 构造一个最小 Claude Messages 请求 JSON（包含 `model`、`max_tokens`、`thinking: {type: enabled, budget_tokens: ...}`，以及带 `tool_use` 的 messages 片段即可）。
  - 调用 Task 1 新增的 helper：
    - case A：`baseModel="kimi-for-coding"` → 断言输出 JSON 中 `thinking` 不存在。
    - case B：`baseModel="claude-sonnet-..."`（任意非 kimi-for-coding）→ 断言输出 JSON 中 `thinking` 仍存在（避免误伤）。
  - 断言用 `gjson.GetBytes(..., "thinking").Exists()` 即可。
  </action>
  <verify>
  go test ./internal/runtime/executor -run TestNormalizeClaude
  </verify>
  <done>
  - 测试稳定通过，且能在未来改动中防止 kimi-for-coding thinking 回归
  - 明确区分“仅 kimi-for-coding 特判”与“其他模型不变”
  </done>
</task>

</tasks>

<verification>
- 自动化：`go test ./...`
- 定向复现（可选）：使用 `.planning` 中记录的失败样本（`logs/error-v1-messages-...`）对应的请求体，确认代理侧转发时已不再包含 `thinking` 字段，从而不再触发“reasoning_content missing”类 400。
</verification>

<success_criteria>
- opencode/Claude Code 走 `/v1/messages` 访问 `kimi-for-coding` 时不再因 thinking + tool calls 触发 400
- 修复范围精确（仅 kimi-for-coding），且通过单测锁定
</success_criteria>

<output>
After completion, create `.planning/quick/002-kimi-for-coding/002-SUMMARY.md`
</output>
