phase: quick-004-kimi-for-coding-reasoning-content
plan: 004
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/runtime/executor/claude_executor.go
  - internal/runtime/executor/claude_executor_test.go
autonomous: true

must_haves:
  truths:
    - "当客户端请求 /v1/messages 且 model=kimi-for-coding 且 thinking.enabled 时，代理会在转发前为历史中的 assistant tool_use 消息补齐 reasoning_content（允许为空字符串），从而避免上游 400"
    - "该修复覆盖非流式与流式（Execute / ExecuteStream），且不改变其他模型的既有行为"
    - "不引入任何新开关/参数；行为仅由 model==kimi-for-coding 决定，并由单元测试锁定"

---

<objective>
在代理层面为 `kimi-for-coding` 增加上游兼容性处理：当请求体开启 `thinking` 时，确保历史里任何“assistant 发起 tool call”的消息包含 `reasoning_content` 字段（哪怕为空字符串），以满足上游服务端校验并避免 400。

Purpose: 保留 thinking 能力，同时修复 OpenCode 在直接 tool-call 时缺失 reasoning_content 的不兼容。
Output: Claude executor 统一转发前归一化逻辑 + 回归测试。
</objective>

<tasks>

<task type="auto">
  <name>Task 1: kimi-for-coding 转发前补齐 tool-call assistant message 的 reasoning_content（非流式 + 流式）</name>
  <files>internal/runtime/executor/claude_executor.go</files>
  <action>
  - 新增一个小的私有 helper（例如 `ensureKimiToolCallReasoningContent(baseModel string, body []byte) []byte`）：
    - 仅当 `baseModel == "kimi-for-coding"` 且 `thinking.type == "enabled"` 时生效
    - 遍历 `messages[]`，对 `role == "assistant"` 且 `content[]` 含 `type == "tool_use"` 的消息：
      - 若 `reasoning_content` 缺失，则补齐为 `""`（空字符串）
  - 在 `ClaudeExecutor.Execute` 与 `ClaudeExecutor.ExecuteStream` 的“最终转发 payload”上调用该 helper
  </action>
  <verify>
  go test ./internal/runtime/executor -run TestEnsureKimiToolCallReasoningContent
  </verify>
</task>

<task type="auto">
  <name>Task 2: 添加回归测试锁定 kimi-for-coding reasoning_content 补齐行为</name>
  <files>internal/runtime/executor/claude_executor_test.go</files>
  <action>
  - case A：kimi-for-coding + thinking.enabled + assistant tool_use（缺失 reasoning_content） => 输出必须存在 reasoning_content 且为 ""
  - case B：已有 reasoning_content 不覆盖
  - case C：非 kimi 模型不变
  - case D：kimi 但未开启 thinking 不变
  - case E：assistant 纯文本消息不变
  </action>
  <verify>
  go test ./internal/runtime/executor
  </verify>
</task>

</tasks>
