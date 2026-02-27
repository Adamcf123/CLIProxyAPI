# Task 003: 创建 beta 参考文档

**Type**: impl
**depends-on**: 无
**Target file**: `docs/claude-beta-reference.md`（新建）

## BDD Scenario

Scenario: beta 参考文档建立手动更新历史
  Given 代理使用手动更新机制维护 OAuth beta 字符串
  When 开发者需要更新 claude_executor.go 中的默认 beta
  Then 可在 docs/claude-beta-reference.md 中查找已验证的版本记录
  And 文档包含更新流程指引（mitmdump 抓包步骤）
  And 文档包含 CLI 2.1.62 的 OAuth beta 初始记录

## Steps

1. 在项目 `docs/` 目录下新建 `claude-beta-reference.md`
2. 写入文档目的：说明此文档为 `claude_executor.go` 中 OAuth 默认 beta 的手动更新参考记录
3. 写入更新流程：
   - 使用 mitmdump 拦截 Claude Code CLI OAuth 直连 api.anthropic.com 的流量
   - 找到 POST /v1/messages 请求
   - 提取其 Anthropic-Beta header 值
   - 用新值更新 `internal/runtime/executor/claude_executor.go` 约第 947 行的 baseBetas 默认赋值
4. 写入版本记录表，字段包含：CLI 版本、抓包日期、Anthropic-Beta 值、备注
5. 添加初始记录：CLI 2.1.62 / 2026-02-27 / `claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28` / mitmdump 实测

## Verification

```bash
# 文档文件存在
ls docs/claude-beta-reference.md

# 包含版本记录
grep "2.1.62" docs/claude-beta-reference.md
```
