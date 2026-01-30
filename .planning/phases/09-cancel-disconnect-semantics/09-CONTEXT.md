# Phase 9 Context: Cancel/Disconnect Semantics

## Why

Milestone v1 audit 提到：客户端 cancel/disconnect 可能不会设置 `LastError`，从而在 Query API 中被误归类为 success，污染聚合与用户判断。

## Audit References

Source: `.planning/v1-MILESTONE-AUDIT.md`

- Tech debt (phase 06): client cancel/disconnect may not set LastError; could be classified as success.

## Success Shape

- canceled 语义在写入/查询/聚合层一致
- Query API 分类不会把 canceled 误判为 success
