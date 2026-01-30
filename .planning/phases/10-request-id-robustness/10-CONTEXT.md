# Phase 10 Context: Request ID Robustness

## Why

Milestone v1 audit 指出 request_id 过短（32-bit），并且 DB 以 PK + `ON CONFLICT DO NOTHING` 去重；若发生碰撞，用户会看到“查不到行”的静默失败。

## Audit References

Source: `.planning/v1-MILESTONE-AUDIT.md`

- Tech debt (milestone): request_id is short (32-bit); collisions manifest as missing rows.

## Success Shape

- request_id 冲突概率显著降低，或冲突可被检测/暴露
- 冲突不再表现为“静默缺行”
