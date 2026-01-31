---
phase: 11-runtime-validation
verified: 2026-01-31T19:45:28Z
status: gaps_found
score: 3/4 must-haves verified
gaps:
  - truth: "报告不包含 secrets；脚本包含 secrets guard"
    status: failed
    reason: "artifacts 证据中出现 raw Authorization header；且 secrets guard 使用 rg 默认忽略 .gitignore，导致对 artifacts/ 的扫描失效"
    artifacts:
      - path: ".planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/logs/error-v1-chat-completions-2026-02-01T032105-297c887a08cd2ecc.log"
        issue: "包含 'Authorization: Bearer ...' header 行（落盘了敏感 header 原文）"
      - path: ".planning/phases/11-runtime-validation/scripts/lib.sh"
        issue: "rg_secrets_guard 使用 rg 默认 ignore 规则，扫描 run_dir 时会跳过 artifacts/ 与 *.log（被 phase-local .gitignore 忽略），导致 guard 实际不生效"
    missing:
      - "在服务端 error log/request dump 中 redact 或删除 Authorization/X-Management-Key 头（避免落盘）"
      - "让 rg_secrets_guard 对 artifacts 生效（例如使用 rg --no-ignore，并避免对 static/management.html 的误报：用 ^Authorization: 锚定 + 排除 static/** 或只扫描日志/文本文件）"
      - "在 report 中补充 'secrets guard verified' 的可审计证据（例如提供一次 scan 输出路径或明确说明 scan 配置与排除项）"
---

# Phase 11: Runtime Validation (Optional) Verification Report

**Phase Goal:** 在真实环境/压测中验证关键 SLO 与边界语义，给发布前信心
**Verified:** 2026-01-31T19:45:28Z
**Status:** gaps_found
**Re-verification:** No (initial verification)

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Phase 11 的验证流程可脚本化复现；产物隔离不污染仓库根目录 `logs/` | ✓ VERIFIED | 脚本存在：`.planning/phases/11-runtime-validation/scripts/run_baseline.sh`、`.planning/phases/11-runtime-validation/scripts/run_edge_cases.sh`；隔离工作目录启动：`.planning/phases/11-runtime-validation/scripts/lib.sh`（`start_server` 里 `cd "$run_dir"`）+ 证据落在 `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/logs/metrics.db` 与 `.planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/logs/metrics.db` |
| 2 | stderr `metrics_summary` 与 SQLite 落库可对照；报告引用 evidence 路径而非粘贴日志 | ✓ VERIFIED | 对照证据：`.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/metrics_summary_sample.txt`（tracking_id）与 `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/sqlite_baseline_checks.txt`（request_id）一致；实现保障：`internal/metricsruntime/request_state.go`（`AttachRequestState` 将 `TrackingID` 对齐 `Gin request_id`）；报告以路径索引：`.planning/phases/11-runtime-validation/11-RUNTIME-VALIDATION-REPORT.md`（Evidence Index） |
| 3 | 覆盖并记录结论：terminal error after headers committed / client cancel (499) / no-usage ensurePublished / persistence degraded observability | ✓ VERIFIED | 主表：`.planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/edge_evidence.tsv`（4 个 scenario 均 >=3 次）；逐次证据：`.planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/sqlite_check_terminal_error_1.tsv`、`.planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/sqlite_check_client_cancel_1.tsv`、`.planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/sqlite_check_no_usage_1.tsv`；degraded 观测：`.planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/management_metrics_persistence_degraded_1.json` |
| 4 | 报告不包含 secrets；脚本包含 secrets guard | ✗ FAILED | 报告本身已提醒不粘贴 header：`.planning/phases/11-runtime-validation/11-RUNTIME-VALIDATION-REPORT.md`；但 artifacts 内出现 raw header：`.planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/logs/error-v1-chat-completions-2026-02-01T032105-297c887a08cd2ecc.log`；且 guard 当前会被 `.planning/phases/11-runtime-validation/.gitignore` 影响而跳过 artifacts/ 与 *.log：`.planning/phases/11-runtime-validation/scripts/lib.sh` |

**Score:** 3/4 truths verified

## Required Artifacts

| Artifact | Expected | Status | Details |
|---------|----------|--------|---------|
| `.planning/phases/11-runtime-validation/scripts/run_baseline.sh` | baseline 可复现 + 采集证据 | ✓ VERIFIED | 具备 `--help`、依赖检查、run_dir 隔离、SQLite snapshot、`metrics_summary` 抽样、secrets guard 调用（但 guard 目前对 artifacts 扫描失效，见 gaps） |
| `.planning/phases/11-runtime-validation/scripts/run_edge_cases.sh` | 4 个 edge scenario 脚本化（>=3 次）+ 证据表 | ✓ VERIFIED | `edge_evidence.tsv` 产物存在且包含 4 场景；`extract_last_request_id` 强制 request_id=16-char hex；degraded 通过 management meta 证据落盘 |
| `.planning/phases/11-runtime-validation/11-RUNTIME-VALIDATION-REPORT.md` | audit-ready 报告（命令 + 阈值 + 结论 + evidence links） | ✓ VERIFIED | 包含命令、阈值、baseline 与 edge 结论、Evidence Index 指向 artifacts 路径 |
| `.planning/phases/11-runtime-validation/artifacts/` | evidence 目录（报告引用的文件实际存在） | ✓ VERIFIED | 例如 `.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/` 与 `.planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/` 均存在并包含 SQLite/日志/表格/management json |

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `.planning/phases/11-runtime-validation/scripts/run_baseline.sh` | `.planning/phases/11-runtime-validation/artifacts/run-*/logs/metrics.db` | `scripts/lib.sh:start_server`（CWD=run_dir） + SQLite snapshot | ✓ WIRED | baseline run_dir 内存在 `logs/metrics.db`：`.planning/phases/11-runtime-validation/artifacts/run-20260131-191633-baseline/logs/metrics.db` |
| `.planning/phases/11-runtime-validation/scripts/run_edge_cases.sh` | `edge_evidence.tsv` + per-run sqlite checks | `metrics_summary` → request_id → `sqlite_row` | ✓ WIRED | `edge_evidence.tsv` 记录 request_id 并指向 sqlite_check 文件路径 |
| `internal/metricsruntime/request_state.go` | stderr `metrics_summary` | `AttachRequestState` 对齐 `TrackingID` | ✓ WIRED | baseline `metrics_summary_sample.txt` 的 tracking_id 与 sqlite request_id 对齐（可审计） |
| `.planning/phases/11-runtime-validation/scripts/lib.sh` | secrets guard | `rg_secrets_guard` | ⚠️ PARTIAL | guard 存在但因 `rg` 默认遵循 `.gitignore` 导致对 artifacts 的扫描无效；且 artifacts 内已有 raw header 落盘 |

## Requirements Coverage

ROADMAP 标注 Phase 11 为 validation-only（无 REQUIREMENTS.md 映射要求）。

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `.planning/phases/11-runtime-validation/artifacts/run-20260131-192103-edge/logs/error-v1-chat-completions-2026-02-01T032105-297c887a08cd2ecc.log` | 9 | `Authorization: Bearer ...` | 🛑 Blocker | artifacts evidence 落盘敏感 header 原文；违背 Phase 11 secrets guard 约束 |
| `.planning/phases/11-runtime-validation/scripts/lib.sh` | 190 | `rg` respects `.gitignore` | 🛑 Blocker | secrets guard 对被 ignore 的 artifacts/ 与 *.log 不生效，无法防止上述泄露 |

## Gaps Summary

当前 Phase 11 的“可审计 runtime validation”主链路（baseline + edge cases + 证据落盘 + 报告引用）整体是存在且可追溯的；但 **secrets guard 目标未达成**：

- 实际 evidence 文件中包含 `Authorization:` header 行，属于不应落盘的敏感信息。
- `rg_secrets_guard` 虽然存在，但因为 `rg` 默认遵循 `.gitignore`，而本 phase 的 `.gitignore` 又忽略了 `artifacts/` 与 `*.log`，导致 guard 在最关键的目录上失效。

建议按 frontmatter gaps 的 `missing` 项完成 gap closure 后，再做一次 re-verification。

---

_Verified: 2026-01-31T19:45:28Z_
_Verifier: Codex (gsd-verifier)_
