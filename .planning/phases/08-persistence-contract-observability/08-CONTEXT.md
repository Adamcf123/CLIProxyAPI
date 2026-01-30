# Phase 8 Context: Persistence Contract & Observability

## Why

Milestone v1 audit 指出持久化链路存在“best-effort 丢行”的设计现实，但目前对外语义与可观测性不足，可能导致用户侧表现为“静默缺行”。

## Audit References

Source: `.planning/v1-MILESTONE-AUDIT.md`

- Tech debt (phase 03): enqueue queue-full/writer-not-started/insert failure 等会 drop 或抑制错误。
- Tech debt (phase 06): ensurePublished 强保证与 persistence best-effort 的张力。

## Success Shape

- 明确并固化 best-effort 的语义契约（何时允许丢、如何被发现）
- 关键 drop 原因可追踪（不弱化安全边界）
