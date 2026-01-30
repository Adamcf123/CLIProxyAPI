# Phase 11 Context: Runtime Validation (Optional)

## Why

Milestone v1 audit 多处建议做运行时/人工验证，以弥补“单元测试难以证明”的关键边界：

- metrics collection 的非阻塞/性能 SLO
- stderr 实时输出与落库在真实环境下的行为
- headers 已提交后发生 terminal error 的边界语义

## Audit References

Source: `.planning/v1-MILESTONE-AUDIT.md`

- Tech debt (phase 02): runtime validation / human verification recommended.
- Tech debt (phase 05): terminal error after headers committed cannot be fully proven by unit tests.

## Success Shape

- 在真实负载下的验证结论可复现（命令/步骤/结果记录）
