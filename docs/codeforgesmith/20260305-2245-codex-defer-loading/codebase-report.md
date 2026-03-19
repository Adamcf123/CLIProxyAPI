# Codebase Report: Codex defer_loading 代理支持

**覆盖置信度**: Solid（两个探查区域均为 Solid）
**关联 Discovery**: `discovery-report.md`（同目录）

---

## 受影响文件

| 文件 | 状态 | 说明 |
|------|------|------|
| `internal/translator/codex/claude/codex_claude_request.go` | 需修改 | 主逻辑文件，全部改动集中于此 |
| `internal/translator/codex/claude/codex_claude_request_test.go` | 需新建 | 当前不存在 |

---

## 区域一：codex_claude_request.go 主逻辑

### 函数入口

`ConvertClaudeRequestToCodex(modelName string, inputRawJSON []byte, _ bool) []byte`（行 36–304）

全程使用 gjson/sjson 直接操作 raw JSON 字符串，无中间结构体。`template` 变量贯穿函数，通过 `sjson.SetRaw` 追加构建输出。

---

### 关键代码块位置

#### messages 处理（行 70–218）

三层嵌套循环：

```
外层 for（行 73）：遍历 messages
  内层 for（行 117）：遍历 message content blocks
    switch contentType（行 121）
      case "text"         → appendTextContent
      case "image"        → 构建 data URL
      case "tool_use"     → flushMessage() + 构建 function_call 消息
      case "tool_result"  → 第三层 for（行 169）：遍历 tool_result content
          if   "image"    → 构建 input_image
          elif "text"     → 构建 input_text
          ← tool_reference 需在此插入新分支（行 196 之后）
```

`tool_reference` 属于第三层循环的新 content block 类型，当前会被静默跳过。

#### tools 数组构建（行 221–263）

两次遍历：
- **第一遍**（行 228–233）：收集所有工具名，构建 `shortMap`
- **第二遍**（行 235–262）：逐工具转换，字段改写顺序：

```
tool_use_id(L243) → type=function(L244) → 短名映射(L246-253)
→ normalizeToolParameters/input_schema→parameters(L255-256)
→ delete parameters.$schema(L257)
→ delete cache_control(L258)
→ delete defer_loading(L259)   ← 当前删除点
→ set strict=false(L260)
→ 追加到 template.tools(L261)
```

**当前问题**：所有工具不区分 `defer_loading` 与否，全部追加到输出。

---

### 新逻辑插入点分析

两处改动，均有天然插入位置：

**改动 A：tools 过滤**

在第二遍遍历（行 235）**之前**，需已知"已加载工具集合"。由于 messages 处理（行 70–218）在 tools 处理（行 221–263）之前，可在函数作用域声明 `loadedTools map[string]bool`，在 messages 处理的 `tool_reference` 分支中填充，在 tools 遍历开头用 `continue` 跳过未加载的 deferred 工具。

**改动 B：tool_reference 转文本**

在第三层循环（行 169–197）的 `else if toolResultContentType == "text"` 之后添加：
```go
else if toolResultContentType == "tool_reference" {
    toolName := contentResult.Get("tool_name").String()
    loadedTools[toolName] = true
    // 追加 text content block: "Tool '<name>' is now available."
}
```

两处改动共享同一个 `loadedTools` 变量，自然耦合，不需要跨函数传递状态。

---

### 可复用的辅助函数

| 函数 | 行号 | 用途 |
|------|------|------|
| `normalizeToolParameters` | 401–417 | raw JSON → 参数 schema 规范化，工具转换时已调用 |
| `shortenNameIfNeeded` | 306–323 | 工具名超长时截断 |
| `buildShortNameMap` | 326–377 | 构建短名映射（第一遍遍历） |

新逻辑不需要新增辅助函数。

---

## 区域二：测试模式

### 当前状态

`internal/translator/codex/claude/` 目录下**无任何测试文件**，需新建 `codex_claude_request_test.go`。

### 项目统一测试模式

从 `gemini/claude`、`gemini-cli/claude`、`openai/claude`、`antigravity/claude` 提取出一致规律：

| 维度 | 规范 |
|------|------|
| 输入数据 | 内联 `[]byte` JSON 字符串字面量，无 testdata 目录或 fixture 文件 |
| 断言方式 | `gjson.GetBytes(output, "path").String()` 等，**不做字符串全等比较，不 Unmarshal 到结构体** |
| 合法性检查 | 边界场景先 `gjson.Valid(string(output))` 再做字段断言 |
| 结构选择 | 简单场景用独立测试函数；多变体用 table-driven（`antigravity/claude` 有完整参考） |

### tool declaration 测试参考

`antigravity/claude` 的 `TestConvertClaudeRequestToAntigravity_ToolDeclarations` 是最接近本次需求的参考：
- 输入带完整 `input_schema` 的工具定义
- 断言输出中 `parameters` 存在、`input_schema` 不存在
- 用于验证字段的转换和删除行为

本次需补充的测试场景见 discovery-report.md 验收标准。

---

## 意外发现

1. `shortenNameIfNeeded`、`buildShortNameMap`、`buildReverseMapFromClaudeOriginalToShort`、`normalizeToolParameters` 四个辅助函数**完全无测试覆盖**——与本次改动无关，但值得记录
2. `tool_result` 第三层循环目前只处理 `image` 和 `text`，`document` 子类型也被静默跳过，`tool_reference` 同理——本次改动可顺带修复这个静默跳过

---

## Architecture Gap Analysis

项目使用自有的 **translator 模式**，与通用分层模板（handler/service/repository）不对应。架构参考模板不适用于本次改动。

| 检查项 | 结论 |
|--------|------|
| 是否需要新建 domain 目录 | 否，改动完全在现有 `codex/claude` 包内 |
| 是否需要新增 service/repository 层 | 否，translator 函数直接转换 JSON，无业务层拆分必要 |
| 是否需要新增共享 utility | 否，`loadedTools` map 是函数作用域内的临时状态，不共享 |
| 是否触碰跨包接口 | 否，`ConvertClaudeRequestToCodex` 签名不变 |

**结论**：本次改动完全符合现有架构模式，无迁移步骤，无 Type 1 架构决策。

## Architecture Migration Steps

无需迁移。

---

## 覆盖空白

无影响本次实现的空白。以下空白不在本次范围内：
- `codex_claude_response.go` 无测试（与本次无关）
- thinking-to-reasoning 分支无测试（与本次无关）
