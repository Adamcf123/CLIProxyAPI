# Discovery Report: Codex defer_loading 代理支持

**日期**: 2026-03-05
**范围**: codex/claude 翻译路径（本次执行）；gemini/claude、gemini-cli/claude（记录，后续执行）

---

## 问题陈述

Claude Code CLI 使用 `advanced-tool-use-2025-11-20` beta 时，会向 Claude 服务端发送携带 `defer_loading` 字段和 `tool_reference` content block 的请求。这两个协议扩展是 Claude 服务端与客户端的专有约定，Codex 不认识它们。

**d26ad82 的现状**：只删除了 `defer_loading` 字段，解决了 400 错误，但更深的机制仍然断裂：
- Codex 收到所有工具 schema（包括本应延迟加载的），无法实现上下文窗口管理
- messages 历史中出现的 `tool_reference` content block，Codex 完全不理解

---

## defer_loading 机制原理（经日志实证）

通过抓取真实请求日志，已确认以下事实：

**协议层**
- 客户端（Claude Code CLI）在 `tools` 数组中发送所有工具的**完整 schema**，deferred 工具额外携带 `defer_loading: true`
- ToolSearch 工具**不带** `defer_loading`，始终在模型上下文中
- 系统消息中注入 `<available-deferred-tools>` 列表，只含工具名，不含 schema

**服务端行为**
- Claude 服务端构建 prompt 时，只将非 deferred 工具的描述注入模型可见上下文
- 模型看到 `<available-deferred-tools>` 列表后，调用 ToolSearch 请求加载工具
- 客户端本地执行 ToolSearch，返回 `{"type": "tool_reference", "tool_name": "X"}` 作为 tool_result
- 下一次请求中，服务端识别 `tool_reference`，将对应工具 schema 注入 prompt

**每次 HTTP 请求对应一次 LLM 推理**，`tool_reference` 的处理发生在 prompt 构建阶段，不触发额外推理。

**关键约束**：deferred 工具在加载前对模型**完全不可见**（无 schema），因此模型无法跳过 ToolSearch 直接调用 deferred 工具——这个"必须先调用 ToolSearch"的约束由协议自然保证，不依赖提示词。

---

## 目标

代理替代 Claude 服务端在 defer_loading 机制中的角色，使 Codex 侧实现与 Claude 完全相同的 deferred tool 加载效果。

---

## 方案

代理在将请求转发给 Codex 前，重建"已加载工具"状态并过滤 tools 数组。

### 状态重建（无副作用，从 messages 历史推导）

扫描请求中的全量 messages，找出所有 `tool_reference` content block，收集已加载的工具名集合。这是无状态操作，因为每次请求都携带完整历史。

### 请求变换

**tools 数组过滤**
```
发送给 Codex 的 tools =
  所有 defer_loading != true 的工具
  + 已被 tool_reference 加载过的 deferred 工具
  （所有工具均去除 defer_loading 字段）
```

**tools 数组预扫描（新增前置步骤）**

在消息循环之前，先对 `tools` 数组做一次预扫描，以工具名为 key 建立查找表：
```
toolSchemaMap: map[string]{description, input_schema}
```
此表仅用于 tool_reference 注入，不影响工具过滤逻辑。

**messages 中 tool_reference 的处理**

将 tool_result 中的 `tool_reference` content block 替换为包含完整 schema 的文本，让 Codex 理解对话历史且获得足够的工具描述信息：

```
// 原始
{"type": "tool_reference", "tool_name": "Read"}

// 转换后（文本内容来自 toolSchemaMap 中 Read 的实际定义）
{"type": "text", "text": "Tool 'Read' is now available.\n\nDescription: <Read 的 description 字段>\n\nParameters:\n<Read 的 input_schema.properties 的 JSON>"}
```

注入内容必须来自请求 `tools` 数组中该工具的原始定义，不得使用占位符或硬编码描述。

**ToolSearch 保留**：转发给 Codex，让 Codex 模型自行决定是否调用。`<available-deferred-tools>` 系统消息同样保留，作为模型行为的引导。

### 数据流示意

```
初始请求（msgs 0-55）
  Codex 看到的 tools: [ToolSearch]   ← 只有非 deferred 工具
  Codex 看到 <available-deferred-tools>: [Bash, Glob, Read, ...]

  Codex 生成 → ToolSearch("select:Read")

客户端执行 ToolSearch → tool_reference{Read}

下一请求（msgs 0-57，含 tool_reference）
  代理预扫描 tools 数组 → 建立 toolSchemaMap
  代理扫描 messages → 发现 Read 已被加载
  代理重写 tool_reference → text 含 Read 完整 description + parameters
  Codex 看到的 tools: [ToolSearch, Read]   ← Read 加入

  Codex 生成 → Read tool_use（有完整上下文，知道参数如何传）

...后续请求类推
```

---

## 验收标准

- [ ] **单元测试**：`ConvertClaudeRequestToCodex` 覆盖以下场景
  - 初始请求：deferred 工具从 tools 数组中过滤掉，非 deferred 工具保留
  - 含 tool_reference 的请求：对应工具出现在 tools 数组中，tool_reference 被替换为文本
  - 多工具场景：多个 deferred 工具按加载顺序逐步出现
- [ ] **端到端测试**：接入真实 Codex，验证完整 ToolSearch→tool_reference→工具调用链路跑通

---

## 不在本次范围内

- gemini/claude 和 gemini-cli/claude 翻译路径（机制相同，后续同步实施）
- 当 Codex 模型本身不遵循 ToolSearch 优先约定时的兜底处理（当前协议保证此情况不会出现）

---

## 开放的不确定性

无。所有关键行为已通过日志实证确认。

---

## 受影响的文件

- `internal/translator/codex/claude/codex_claude_request.go`（主要改动）
- `internal/translator/codex/claude/codex_claude_request_test.go`（新增/补充测试）
