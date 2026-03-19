# Implementation Plan: Codex defer_loading 代理支持

## 实现路径

预扫描建表在前，消息循环中填充加载状态并注入 schema，最后过滤 tools 数组——三步共享同一批局部 map，按数据依赖顺序串行执行，测试在逻辑完成后新建。

---

## 任务列表

## ✓ #1 — 预扫描 tools 数组，建立共享 map

**涉及文件**: `internal/translator/codex/claude/codex_claude_request.go`
**依赖**: 无

**说明**: 在 `ConvertClaudeRequestToCodex` 函数内，`rootResult` 解析完成后（行 41）、系统消息循环（行 45）之前，对 `tools` 数组做一次预扫描，建立两个函数作用域内的局部 map，供后续任务共享：
- `toolSchemaMap map[string]struct{ description, inputSchema string }`：工具名 → 完整 schema 信息
- `deferredToolNames map[string]bool`：值为 `true` 的工具名即带 `defer_loading: true`
- `loadedTools map[string]bool`：初始为空，由 #2 填充

工具名使用请求中的**原始名**（`tool_reference` 里的 `tool_name` 与之对应）。

**验收标准**:
- [x] 三个 map 在函数开头声明，作用域覆盖消息循环和 tools 循环
- [x] 对每个 tool，若 `defer_loading` 字段为 `true`，则 `deferredToolNames[name] = true`
- [x] 对每个 tool，`toolSchemaMap[name].description` 和 `toolSchemaMap[name].inputSchema` 保存对应字段的原始值（inputSchema 保存 `input_schema` 字段的 raw JSON）
- [x] 预扫描只读不写 `template`，不影响现有输出

---

## ✓ #2 — 消息循环中处理 tool_reference

**涉及文件**: `internal/translator/codex/claude/codex_claude_request.go`
**依赖**: #1

**说明**: 在 tool_result 第三层循环（行 169–197）的 `else if toolResultContentType == "text"` 分支之后，新增 `else if toolResultContentType == "tool_reference"` 分支：从 `toolSchemaMap` 查找该工具的完整定义，构造包含 description 和 parameters 的文本，以 `input_text` 类型追加到 `toolResultContent`，并将工具名写入 `loadedTools`。

注入文本格式（固定，测试将精确断言）：

```
Tool '<name>' is now available.

Description: <description>

Parameters:
<input_schema.properties 的 raw JSON，若 properties 不存在则使用完整 input_schema raw JSON>
```

若 `toolSchemaMap` 中找不到该工具名（不应出现，但需防御），仅注入 `Tool '<name>' is now available.`。

**验收标准**:
- [x] `tool_reference` 类型不再静默跳过，不再进入 fallback 路径（`output` 不会是包含 `tool_reference` 原始字符串的 JSON）
- [x] 注入文本的第一行为 `Tool '<name>' is now available.`
- [x] 注入文本包含 `Description:` 段（若工具有 description）
- [x] 注入文本包含 `Parameters:` 段，内容为该工具 `input_schema.properties` 的 JSON（若 properties 不存在，回退到完整 `input_schema`）
- [x] `loadedTools[toolName] = true` 在处理 tool_reference 时被设置
- [x] `toolResultContentIndex` 正确递增

---

## ✓ #3 — tools 第二遍过滤未加载的 deferred 工具

**涉及文件**: `internal/translator/codex/claude/codex_claude_request.go`
**依赖**: #1, #2

**说明**: 在 tools 第二遍遍历（行 235）的循环体开头，获取当前工具的原始名，判断：若该名在 `deferredToolNames` 中且**不在** `loadedTools` 中，则 `continue` 跳过，不追加到 `template.tools`。其余工具（非 deferred 工具、已加载的 deferred 工具）正常处理，保留现有字段转换逻辑不变。

注意：工具名在此时尚未做短名映射（shortMap 在行 234 刚建好），过滤判断应使用 `toolResult.Get("name").String()` 的原始名，与 `deferredToolNames` 和 `loadedTools` 保持一致。

**验收标准**:
- [x] 带 `defer_loading: true` 且未出现在任何 `tool_reference` 中的工具，不出现在输出 `tools` 数组中
- [x] 带 `defer_loading: true` 且已在 `loadedTools` 中的工具，正常出现在输出 `tools` 数组中（short name 映射、parameters 转换等逻辑不变）
- [x] 不带 `defer_loading` 的工具不受影响，全部保留在输出 `tools` 数组中
- [x] `defer_loading` 字段在输出中仍被删除（现有行 259 逻辑不变）

---

## ✓ #4 — 新建单元测试文件

**涉及文件**: `internal/translator/codex/claude/codex_claude_request_test.go`（新建）
**依赖**: #1, #2, #3

**说明**: 新建测试文件，package 为 `claude`，遵循项目惯例（gjson 路径断言 + 内联 `[]byte` JSON 字面量，不做整体字符串比较，不 Unmarshal 到结构体）。覆盖三个独立测试函数：

**测试函数一**：`TestConvertClaudeRequestToCodex_DeferLoading_InitialRequest`

输入：含两个工具（ToolSearch 不带 `defer_loading`；Read 带 `defer_loading: true`），messages 无 `tool_reference`。

验收标准：
- [x] 输出 `tools` 数组长度为 1
- [x] 输出 `tools.0.name` 为 ToolSearch（或其短名）
- [x] 输出 `tools` 中不含 Read

---

**测试函数二**：`TestConvertClaudeRequestToCodex_DeferLoading_WithToolReference`

输入：同上两个工具，messages 中含一条 tool_result，其 content 为 `[{"type":"tool_reference","tool_name":"Read"}]`。

验收标准：
- [x] 输出 `tools` 数组长度为 2（ToolSearch + Read 均出现）
- [x] `function_call_output` 的 `output` 字段不含原始 `tool_reference` 字符串
- [x] `output.0.type` 为 `input_text`
- [x] `output.0.text` 以 `Tool 'Read' is now available.` 开头
- [x] `output.0.text` 包含 `Description:` 子串（若输入工具有 description）
- [x] `output.0.text` 包含 `Parameters:` 子串（若输入工具有 input_schema）

---

**测试函数三**：`TestConvertClaudeRequestToCodex_DeferLoading_MultipleTools`

输入：三个工具（ToolSearch 非 deferred；Read 和 Bash 均带 `defer_loading: true`），messages 中仅含 Read 的 tool_reference。

验收标准：
- [x] 输出 `tools` 包含 ToolSearch 和 Read
- [x] 输出 `tools` 不含 Bash

---

## ✓ #5 — 单元测试：同一 deferred 工具被 tool_reference 两次

**涉及文件**: `internal/translator/codex/claude/codex_claude_request_test.go`
**依赖**: #4

**说明**: 验证在多轮对话中同一 deferred 工具被 tool_reference 两次时，工具过滤逻辑仍正确（工具只出现一次），且两次 tool_result 的 output 均完成 schema 注入。此场景是多轮长对话的常见路径：模型每次推理前客户端重新发送全量历史，历史里可能有两条 tool_reference{Read}。

**当前代码行为**：每次遇到 `tool_reference` 都执行注入，`loadedTools[name] = true` 幂等；tools 过滤仅检查 `loadedTools` 是否存在，因此 Read 只出现一次。schema 文本在两条 tool_result 中各出现一次。

**验收标准**:
- [x] 输出 `tools` 数组长度为 2（ToolSearch + Read），Read 不重复出现
- [x] 第一条 `function_call_output.output.0.text` 以 `Tool 'Read' is now available.` 开头
- [x] 第二条 `function_call_output.output.0.text` 也以 `Tool 'Read' is now available.` 开头（schema 文本注入两次，记录当前行为）
- [x] 输出 JSON 合法（`gjson.Valid` 通过）

---

## ✓ #6 — 单元测试：tool_reference 引用不在 tools 数组中的工具名

**涉及文件**: `internal/translator/codex/claude/codex_claude_request_test.go`
**依赖**: #4

**说明**: 验证 `toolSchemaMap` 中找不到对应工具名时的防御路径。此场景在协议层不应出现，但异常请求或客户端 bug 可能触发，需确认不会 panic 且输出合法。

**当前代码行为**：`if entry, ok := toolSchemaMap[toolName]; ok { ... }` 分支不进入，仅注入 `Tool 'X' is now available.`（无 Description/Parameters 段）。`loadedTools["UnknownTool"] = true` 但 UnknownTool 不在原始 tools 数组中，因此输出 tools 中不会出现该工具。

**验收标准**:
- [x] 不 panic，程序正常返回
- [x] 输出 JSON 合法（`gjson.Valid` 通过）
- [x] `function_call_output.output.0.type` 为 `input_text`
- [x] `function_call_output.output.0.text` 等于 `Tool 'UnknownTool' is now available.`（不含 `Description:` 或 `Parameters:` 段）
- [x] 输出 `tools` 数组只含 ToolSearch，UnknownTool 不出现

---

## ✓ #7 — 单元测试：全部工具均为 deferred 且无任何 tool_reference

**涉及文件**: `internal/translator/codex/claude/codex_claude_request_test.go`
**依赖**: #4

**说明**: 验证当 tools 数组中所有工具都带 `defer_loading: true`、messages 中没有 tool_reference 时，过滤后输出 tools 为空数组，且整体 JSON 结构合法。实际场景中 ToolSearch 不带 defer_loading 因而不会触发，但此测试为假设性边界，确认空 tools 路径不会 panic 或产生非法 JSON（如 `"tools":null`）。

**当前代码行为**：`toolsResult.IsArray()` 为 true，执行 `sjson.SetRaw(template, "tools", []`)` 后，所有工具均被 `continue` 跳过，最终 `tools: []`，`tool_choice: "auto"` 仍被写入。

**验收标准**:
- [x] 不 panic，程序正常返回
- [x] 输出 JSON 合法（`gjson.Valid` 通过）
- [x] `gjson.GetBytes(output, "tools").IsArray()` 为 true 且数组长度为 0（不是 null）
- [x] `gjson.GetBytes(output, "tool_choice").String()` 为 `auto`
- [x] `gjson.GetBytes(output, "parallel_tool_calls").Bool()` 为 true

---

## ✓ #8 — 单元测试：tool_reference 与 text 在同一 tool_result 的 content 中混合

**涉及文件**: `internal/translator/codex/claude/codex_claude_request_test.go`
**依赖**: #4

**说明**: 验证 tool_result 的 content 数组同时包含 text 和 tool_reference 两种类型时，`toolResultContentIndex` 正确递增，两个内容块均被转换并出现在输出 output 数组的对应位置，不互相覆盖也不丢失。这是 ToolSearch 返回文本摘要同时触发 tool_reference 的真实路径。

**当前代码行为**：第三层循环对 text 和 tool_reference 分支各自递增 `toolResultContentIndex`，两者位于同一 `toolResultContent` JSON 数组的相邻位置。

**验收标准**:
- [x] `function_call_output.output` 数组长度为 2
- [x] `output.0.type` 为 `input_text`，`output.0.text` 等于 `search done`（text 块）
- [x] `output.1.type` 为 `input_text`，`output.1.text` 以 `Tool 'Read' is now available.` 开头（tool_reference 块）
- [x] `output.1.text` 包含 `Description:` 和 `Parameters:` 段
- [x] Read 出现在输出 `tools` 数组中（已被标记为已加载）
- [x] 输出 JSON 合法（`gjson.Valid` 通过）

---

<!-- VERIFY GAP: e2e 测试（接入真实 Codex 验证完整 ToolSearch→tool_reference→工具调用链路）未形成自动化测试。discovery-report.md 验收标准明确列出此项。当前状态：已通过真实流量日志手工验证（2026-03-06，32 条 Codex 请求全部 HTTP 200，过滤和注入行为均正确）。若需自动化，建议以 integration test tag 单独管理，需要真实 Codex 端点和认证。 -->
