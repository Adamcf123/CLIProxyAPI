# Critical Design Report: Codex defer_loading 代理支持

---

## 变更全景

实施后，CLIProxyAPI 在将 Claude Code CLI 请求转发给 Codex 时，将完整承接原本由 Claude 上游服务端负责的 defer_loading 机制。具体表现为：Codex 在每次请求中只看到"当前已加载"的工具 schema，而非全部工具；当模型调用 ToolSearch 加载某个工具时，代理识别返回的 `tool_reference` content block，将对应工具加入后续请求的 tools 数组，并把 `tool_reference` 转换为 Codex 可理解的文本消息。调用方（Claude Code CLI）无感知，整条 ToolSearch → tool_reference → 工具调用链路在 Codex 侧能够正常跑通。新建 `codex_claude_request_test.go` 覆盖该逻辑的关键分支。

---

## 已确认的关键决策

### D1：tool_reference 转换后注入包含完整 schema 的文本

**决定内容**：将 messages 历史中 `tool_result` 内的 `tool_reference` content block 替换为文本内容块，文本内容包含该工具的完整 description 和 parameters，来源必须是请求 `tools` 数组中的原始定义：

```json
{
  "type": "text",
  "text": "Tool 'Read' is now available.\n\nDescription: <Read 的 description>\n\nParameters:\n<Read 的 input_schema.properties JSON>"
}
```

实现上需要在函数开头对 `tools` 数组做一次预扫描，建立 `toolName → {description, input_schema}` 查找表，在 tool_reference 分支中查表注入。

**为什么是承诺**：
1. 注入格式（字段顺序、换行、标题措辞）一旦确定，测试用例将精确断言，后续修改需同步更新测试
2. 后续 gemini/gemini-cli 路径若复用同一格式，改动成本放大至三条路径
3. 预扫描步骤改变了函数内部数据流顺序，是结构性决定

**考虑过的替代方案**：
- 仅写入 `"Tool 'X' is now available."`（无 schema）：模型上下文缺少 description 和 parameters，无法判断工具用途和参数传递方式，功能正确但效果退化
- 空 content（静默丢弃）：模型不感知加载事件，无法从历史判断工具是否可用
- 删除整条 tool_result 消息：历史干净但 tool_use_id 孤立，违反 OpenAI 消息协议要求

**选择理由**：完整 schema 文本使 Codex 模型在对话历史中"看到"与 Claude 模型相同的工具信息，行为最接近原生 Claude 路径；仅写入可用通知会使模型缺少参数信息，无法可靠地调用复杂工具。

---

## 跳过的决策（Type 2，可逆）

- **注入文本的具体格式细节**：标题换行数、parameters 是否包含完整 input_schema 还是只有 properties——遵循现有模式，随时可调整
- **测试断言方式**：遵循项目现有惯例（gjson 路径断言 + 内联 JSON 字面量），无需决策，随时可重构
- **toolSchemaMap 的数据结构**：函数作用域内的临时 map，内部实现细节
- **gemini/gemini-cli 路径复用抽象**：延后到该路径实施时决定，本次独立实现即可
