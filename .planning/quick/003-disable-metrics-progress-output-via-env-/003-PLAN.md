---
phase: quick-003-disable-metrics-progress-output-via-env
plan: 003
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
    - "当环境变量 CLIPROXY_METRICS_PROGRESS_DISABLED=1(或 true/yes) 时，即使 stderr 为 TTY，也不会输出任何实时 progress 覆盖行"
    - "无论是否禁用 progress，请求结束仍会在 stderr 输出单行 metrics_summary JSON（可检索、可对账）"
  artifacts:
    - path: "internal/metricsruntime/display.go"
      provides: "progress 输出在入口处可被 env 强制禁用，但不影响 metrics_summary"
    - path: "internal/metricsruntime/display_test.go"
      provides: "覆盖 env 禁用 progress + 保留 metrics_summary 的回归测试"
    - path: "重要命令.txt"
      provides: "对外说明如何通过 env 禁用实时 progress"
  key_links:
    - from: "internal/metricsruntime/display.go"
      to: "CLIPROXY_METRICS_PROGRESS_DISABLED"
      via: "PrintProgress 入口 gate"
      pattern: "CLIPROXY_METRICS_PROGRESS_DISABLED"
    - from: "internal/metricsruntime/display_test.go"
      to: "internal/metricsruntime/display.go"
      via: "t.Setenv + 断言 progress 静默 / summary 仍输出"
      pattern: "Setenv\(.*CLIPROXY_METRICS_PROGRESS_DISABLED"
---

<objective>
为 CLIProxy 的指标展示增加一个仅靠环境变量控制的“强制静默实时 progress 行”能力，避免在需要可读日志/录屏/集成工具时出现不断刷新的覆盖行；同时严格保留请求结束的 metrics_summary 单行证据输出。

Purpose: 让用户可以在交互式 TTY 场景按需关闭 live progress，而不牺牲最终可检索的指标证据。
Output: 代码层面新增 env gate + 回归测试 + 对外说明。
</objective>

<execution_context>
@~/.config/opencode/get-shit-done/workflows/execute-plan.md
@~/.config/opencode/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@.planning/quick/001-remove-live-metrics-printing/001-SUMMARY.md

@internal/metricsruntime/display.go
@internal/metricsruntime/display_test.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: 在 PrintProgress 增加 env 强制禁用 gate（不影响 summary）</name>
  <files>internal/metricsruntime/display.go</files>
  <action>
实现环境变量 `CLIPROXY_METRICS_PROGRESS_DISABLED`：当其为 truthy（至少支持 `1`/`true`/`yes`，大小写不敏感）时，实时 progress 覆盖行完全静默。

约束：
- 只影响 live progress 行（即 `PrintProgress(...)` 或等价入口），不得改变 `metrics_summary` 最终单行输出的任何行为/格式。
- 保持现有的 TTY gate：非 TTY 仍然不输出 progress；env gate 是在“本来会输出 progress 的情况下”进一步强制禁用。
- 避免在热路径反复做复杂解析：允许读取 env，但解析逻辑必须简单、可测；如已有统一的 env bool 工具函数，复用并保持单一事实来源。
  </action>
  <verify>
go test ./... （或最小化到包含 `internal/metricsruntime` 的 package 集）
  </verify>
  <done>
设置 `CLIPROXY_METRICS_PROGRESS_DISABLED=1` 时，即使 `isTTY=true`，调用 progress 打印函数也不会向 writer 写入任何覆盖行；summary 相关函数不受影响。
  </done>
</task>

<task type="auto">
  <name>Task 2: 添加回归测试：env 禁用 progress 但保留 metrics_summary</name>
  <files>internal/metricsruntime/display_test.go</files>
  <action>
新增/扩展测试覆盖以下组合：
- `isTTY=true` 且 `CLIPROXY_METRICS_PROGRESS_DISABLED=1`：progress 输出应为空（不包含 `\r`、不包含 ANSI clear、也不包含任何 progress 文本）。
- 在同一测试中（或紧邻测试），调用输出 `metrics_summary` 的路径（例如 stop/summary 入口）仍应产出单行 `metrics_summary {json}`（沿用 quick-001 已锁定的契约）。

实现要求：
- 使用 Go 的 `t.Setenv(...)`，避免污染全局环境。
- 测试断言要基于“输出内容是否出现”而不是时间/计时，确保稳定。
  </action>
  <verify>
go test ./... （确保测试在本机/CI 可稳定通过）
  </verify>
  <done>
测试在启用 env gate 时稳定证明：progress 静默 + metrics_summary 保留。
  </done>
</task>

<task type="auto">
  <name>Task 3: 更新运行说明：记录 env 禁用 progress 的用法</name>
  <files>重要命令.txt</files>
  <action>
在与 metrics 输出说明相邻的位置补充一条“可通过环境变量强制禁用实时 progress 行”的说明，包含：
- env 名称：`CLIPROXY_METRICS_PROGRESS_DISABLED`
- 示例：`CLIPROXY_METRICS_PROGRESS_DISABLED=1 ./cliproxy ...`
- 明确不会影响 `metrics_summary` 单行输出。

注意：不要引入新的 CLI 参数（本需求仅允许 env bool）。
  </action>
  <verify>
人工快速检查该说明可读、位置合理；同时 `go test ./...` 仍通过。
  </verify>
  <done>
用户可从 `重要命令.txt` 直接找到该 env 用法并理解其作用边界（只禁用 progress，不影响 summary）。
  </done>
</task>

</tasks>

<verification>
- `CLIPROXY_METRICS_PROGRESS_DISABLED=1` + stderr 为 TTY：运行一次请求后，过程不出现覆盖式 progress 行，但仍能看到 `metrics_summary {...}` 单行输出。
- 默认不设置该 env：行为与当前一致（TTY 下 progress 仍可见；非 TTY 下 progress 仍静默）。
</verification>

<success_criteria>
- env gate 生效且只影响 progress 行
- metrics_summary 合同保持不变（内容与格式不被修改）
- 回归测试覆盖并通过
</success_criteria>

<output>
After completion, create `.planning/quick/003-disable-metrics-progress-output-via-env-/003-SUMMARY.md`
</output>
