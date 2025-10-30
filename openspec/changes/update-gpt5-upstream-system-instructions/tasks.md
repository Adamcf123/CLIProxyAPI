## 1. Tests (先测后码)
- [ ] Responses→Codex：gpt-5-* 不注入 IGNORE，不写入内置 instructions，instructions=上游 system（顶层/消息内），无则空
- [ ] Chat Completions→Codex：同上，覆盖字符串与 text 对象两种 system 表达
- [ ] Claude→Codex：同上，覆盖顶层与消息内 system 两种来源
- [ ] 非 gpt-5(-codex) 基线用例保持现状

## 2. Implementation
- [ ] 新增 `isGpt5Family(model string) bool`（正则 `^gpt-5(-codex)?-.*$`）
- [ ] Responses：命中时跳过 IGNORE 注入与内置 instructions；提取上游 system→instructions
- [ ] Chat Completions：命中时不写入内置 instructions；提取 messages 中 system→instructions
- [ ] Claude：命中时跳过 IGNORE 注入与内置 instructions；提取上游 system→instructions

## 3. Regression & Docs
- [ ] 回归非命中模型的现有行为（含 HasPrefix 例外）
- [ ] 更新变更备注（本 proposal 即记录）
