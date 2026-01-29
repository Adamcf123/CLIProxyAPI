# Phase 7 Context: Docs & Traceability Cleanup

## Why

Milestone v1 audit 标记为 `tech_debt`，核心原因之一是规划文档与已验证实现存在漂移：

- `REQUIREMENTS.md` 中 METR-01..04 在 traceability/checklist 仍显示 Pending，但 Phase 1 verification 已确认 satisfied。
- 部分文档/命令仍引用 legacy JSONL 路径，尽管 SQLite 已是指标数据的单一来源。

## Audit References

Source: `.planning/v1-MILESTONE-AUDIT.md`

- Tech debt (milestone): REQUIREMENTS drift, legacy docs reference JSONL.

## Success Shape

- Requirements/traceability 与 phase verification 对齐
- 文档中的数据源与命令路径对齐到 SQLite
