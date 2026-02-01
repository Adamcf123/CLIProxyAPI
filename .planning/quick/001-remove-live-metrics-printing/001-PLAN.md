---
phase: quick-001-remove-live-metrics-printing
plan: 001
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/metricsruntime/display.go
  - internal/metricsruntime/display_test.go
  - 重要命令.txt
autonomous: true

must_haves:
  truths:
    - "当 stderr 不是 TTY（重定向/CI/pipe）时，不输出每秒 metrics 进度行，避免日志被刷屏或夹杂控制字符"
    - "请求结束仍输出单行 metrics_summary JSON 到 stderr，便于检索与对账"
    - "当 stderr 是 TTY 时，仍可显示覆盖式实时进度（\\r + ANSI clear），且不影响 HTTP 响应体（stdout）"
  artifacts:
    - path: "internal/metricsruntime/display.go"
      provides: "live display 的 TTY 检测与输出策略（progress + summary）"
    - path: "internal/metricsruntime/display_test.go"
      provides: "非 TTY 场景不输出进度的回归测试"
    - path: "重要命令.txt"
      provides: "运行说明与指标输出行为描述"
  key_links:
    - from: "internal/metricsruntime/display.go"
      to: "os.Stderr"
      via: "PrintProgress() / PrintSummary()"
      pattern: "fmt\\.F(print|printf|println)\\(os\\.Stderr"
    - from: "sdk/api/handlers/*"
      to: "internal/metricsruntime/StartLiveDisplay"
      via: "request handler defer stop()"
      pattern: "metricsruntime\\.StartLiveDisplay\\(state\\)"
---

<objective>
移除非交互式环境（stderr 非 TTY）下的 live metrics 实时进度输出，让日志在重定向/CI/聚合采集时保持可读，同时保留请求结束时的单行 metrics_summary JSON。

Purpose: 解决“metrics 输出污染日志不可读”问题，同时不引入新的开关/配置项。
Output: 调整后的 live display 输出策略 + 回归测试 + 文档说明同步。
</objective>

<execution_context>
@~/.config/opencode/get-shit-done/workflows/execute-plan.md
@~/.config/opencode/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@internal/metricsruntime/display.go
@internal/metricsruntime/request_state.go
@sdk/api/handlers/openai/openai_handlers.go
@重要命令.txt
</context>

<tasks>

<task type="auto">
  <name>Task 1: 仅在 TTY 输出实时进度，非 TTY 静默</name>
  <files>internal/metricsruntime/display.go</files>
  <action>
  将“实时进度行（metrics tracking=...）”变为严格的 TTY-only 行为：
  - 在 `PrintProgress(state, isTTY bool)` 入口添加 fail-fast：`if !isTTY { return }`。
    - 目的：避免在 stderr 被重定向/CI/pipe 时产生刷屏日志与控制字符污染。
    - 要求：不改变 `PrintSummary()` 的输出（仍然在 stop 时打印单行 `metrics_summary` JSON）。
  - 不新增 CLI flags、环境变量或配置项来控制是否输出 metrics（避免业务逻辑参数化）。
  </action>
  <verify>
  go test ./...
  </verify>
  <done>
  - `PrintProgress(..., false)` 不向 stderr 写任何内容
  - `StartLiveDisplay(...).stop()` 仍输出单行 `metrics_summary {...}` 到 stderr
  </done>
</task>

<task type="auto">
  <name>Task 2: 添加非 TTY 无进度输出的回归测试</name>
  <files>internal/metricsruntime/display_test.go</files>
  <action>
  新增单元测试覆盖关键行为，避免后续改动把“进度行”重新带回日志：
  - 创建 `internal/metricsruntime/display_test.go`。
  - 用 `os.Pipe()` 临时替换 `os.Stderr`，调用：
    - `state := NewRequestState(true, "test-model")`
    - `PrintProgress(state, false)`
  - 读取 pipe 内容，断言输出为空字符串。
  - 测试结束必须恢复原 `os.Stderr`（用 `t.Cleanup` 或 `defer`）。
  </action>
  <verify>
  go test ./internal/metricsruntime -run TestPrintProgress
  </verify>
  <done>
  - 测试在本机与 CI 均稳定通过
  - 明确锁定“非 TTY 无进度输出”的契约
  </done>
</task>

<task type="auto">
  <name>Task 3: 同步运行说明中的指标输出描述</name>
  <files>重要命令.txt</files>
  <action>
  更新 `重要命令.txt` 中对 Phase 02 指标输出的描述：
  - 将“流式请求期间 stderr 进度行”改为“仅在 stderr 为 TTY 时显示进度行；非 TTY 仅输出结束时 metrics_summary”。
  - 保留 metrics_summary 单行 JSON 的说明（这是可检索、可对账的关键证据）。
  </action>
  <verify>
  rg -n "Phase 02 指标输出" 重要命令.txt
  </verify>
  <done>
  - 文档描述与实际行为一致
  </done>
</task>

</tasks>

<verification>
- 自动化：`go test ./...`
- 手动抽查（可选）：启动服务并将 stderr 重定向到文件/管道，确认没有 `metrics tracking=`，且仍存在单行 `metrics_summary`。
</verification>

<success_criteria>
- 在日志采集/CI/重定向场景下，stderr 不再被每秒 metrics 进度行刷屏
- 每个请求仍能在结束时得到单行 `metrics_summary` JSON（可搜索、可关联 tracking_id）
</success_criteria>

<output>
After completion, create `.planning/quick/001-remove-live-metrics-printing/001-SUMMARY.md`
</output>
