phase: quick-005-metrics-streaming-content-detection
plan: 005
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/metricsruntime/request_state.go
  - internal/metricsruntime/request_state_test.go
  - internal/metricsruntime/usage_plugin.go
autonomous: true

must_haves:
  truths:
    - "当上游为 OpenAI Responses/Codex streaming 时，metricsruntime 能识别 response.output_text.delta 作为内容输出，从而可以记录 TTFT 并减少 TPS/TPOT 为空的比例"
    - "当上游为 Claude/Kimi tool streaming 时，metricsruntime 将 input_json_delta.partial_json 视为内容输出，从而 tool-heavy 请求也能产生 TTFT/TPS/TPOT"
    - "SSE chunk 中包含多个 data: event 时，ContentTokenChunks 会按 event 计数，避免被合并写导致的 false-negative gating"
    - "保持置信度门槛：仍避免 near-zero duration 导致的极端 TPS；仅将 streaming post-first-token 最小持续时间从 300ms 调整为 50ms"

---

<objective>
修复 metrics.db / metrics_summary 中 streaming 指标（ttft/tps/tpot）大量为 null 的可用性问题：
1) 补齐 OpenAI Responses/Codex 的内容事件识别；
2) 将 kimi-for-coding 的工具参数增量输出也纳入“内容输出”判定；
3) 修正 SSE 合并写导致的 chunk 计数偏小。

Purpose: 让 TPS/TTFT/TPOT 在主流流式协议下“可计算且可置信”，而不是长期全是 null。
Output: RequestState 内容识别 + 流式计数改进 + 置信度门槛微调 + 单元测试。
</objective>

<tasks>

<task type="auto">
  <name>Task 1: 扩展流式内容事件识别（OpenAI Responses + Claude tool streaming）</name>
  <files>internal/metricsruntime/request_state.go</files>
  <action>
  - 在 jsonHasContentToken 中新增：
    - OpenAI Responses: type=response.output_text.delta 且 delta 非空 => 视为内容
    - Claude tool streaming: type=content_block_delta 且 delta.type=input_json_delta 且 delta.partial_json 非空 => 视为内容
  - 将 SSE chunk 的内容计数改为按 data: event 计数（同一 chunk 内多个 data: 逐个计数）
  </action>
  <verify>
  go test ./internal/metricsruntime -run TestMaybeRecordFirstContentToken
  </verify>
</task>

<task type="auto">
  <name>Task 2: 调整 streaming 置信度门槛，降低短响应被误判为不可计算的比例</name>
  <files>internal/metricsruntime/usage_plugin.go</files>
  <action>
  - 将 minStreamingPostFirstTokenDuration 从 300ms 调整到 50ms（仍保留极端 TPS 防护）
  </action>
  <verify>
  go test ./internal/metricsruntime
  </verify>
</task>

</tasks>
