---
phase: 07-docs-traceability-cleanup
plan: 04
subsystem: docs
tags: [sdk-docs, sqlite, metrics, management-api, audit, traceability]

# Dependency graph
requires:
  - phase: 03-persistence
    provides: SQLite metrics persistence at logs/metrics.db
  - phase: 04-query-api
    provides: GET /v0/management/metrics query endpoint
provides:
  - operator-facing SDK docs no longer imply JSONL as a metrics source
  - SDK usage docs explicitly point to logs/metrics.db and /v0/management/metrics when discussing metrics storage
affects: [07-docs-traceability-cleanup, audit, operations, docs]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - docs drift cleanup guarded by executable text checks (python snippets)

key-files:
  created:
    - .planning/phases/07-docs-traceability-cleanup/07-04-SUMMARY.md
  modified:
    - docs/sdk-usage.md
    - docs/sdk-usage_CN.md

key-decisions:
  - "None - followed plan as specified"

patterns-established:
  - "Docs: metrics persistence source-of-truth is SQLite logs/metrics.db"

# Metrics
duration: 1 min
completed: 2026-01-30
---

# Phase 7 Plan 04: SDK Docs JSONL Drift Cleanup Summary

**SDK operator docs no longer imply JSONL metrics sources; when metrics storage is mentioned, it points to SQLite `logs/metrics.db` (or `GET /v0/management/metrics`).**

## Performance

- **Duration:** 1 min
- **Started:** 2026-01-30T16:46:55Z
- **Completed:** 2026-01-30T16:48:06Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- Audited the 8 SDK operator docs (`docs/sdk-*.md` and `docs/sdk-*_CN.md`) for legacy JSONL wording and removed any drift risk.
- Clarified metrics storage and query entrypoints in the SDK usage guides without adding new behavioral promises.

## What Changed (by file)

### `docs/sdk-usage.md`

- Added an explicit statement that metrics are persisted to SQLite at `logs/metrics.db`, with a query entrypoint at `GET /v0/management/metrics`.

### `docs/sdk-usage_CN.md`

- 同步补充指标数据的持久化与查询入口：SQLite `logs/metrics.db` 与 `GET /v0/management/metrics`。

## Verify

Task 1 verification (SDK docs contain no JSONL references, and storage references point to SQLite / management API):

```bash
python - <<'PY'
import pathlib
import sys

needles = ['.jsonl', 'jsonl', 'metrics-yyyy']
persist_signals = ['persist', 'persistence', 'storage', 'database', 'sqlite', '落库', '落盘', '数据库']
metrics_signals = ['metrics', 'tps', 'ttft', 'tpot', '指标']
must_point_to = ['logs/metrics.db', 'v0/management/metrics']
paths = [
    pathlib.Path('docs/sdk-access.md'),
    pathlib.Path('docs/sdk-access_CN.md'),
    pathlib.Path('docs/sdk-advanced.md'),
    pathlib.Path('docs/sdk-advanced_CN.md'),
    pathlib.Path('docs/sdk-usage.md'),
    pathlib.Path('docs/sdk-usage_CN.md'),
    pathlib.Path('docs/sdk-watcher.md'),
    pathlib.Path('docs/sdk-watcher_CN.md'),
]

for p in paths:
    if not p.exists():
        print(f"missing expected docs file: {p}")
        sys.exit(1)

    text = p.read_text(encoding='utf-8').lower()
    for n in needles:
        if n in text:
            print(f"unexpected '{n}' mention in {p}")
            sys.exit(1)

    # Only enforce SQLite pointer if the doc discusses *metrics persistence/storage*.
    has_metrics = any(s in text for s in metrics_signals)
    has_persist = any(s in text for s in persist_signals)
    if has_metrics and has_persist:
        if not any(m in text for m in must_point_to):
            print(f"{p} mentions metrics persistence/storage but does not reference logs/metrics.db or /v0/management/metrics")
            sys.exit(1)

print('ok')
PY
```

Task 2 verification (summary includes required audit hooks):

```bash
rg -nS "Phase 7" .planning/phases/07-docs-traceability-cleanup/07-04-SUMMARY.md
rg -nS "docs/" .planning/phases/07-docs-traceability-cleanup/07-04-SUMMARY.md
```

## Task Commits

Each task was committed atomically:

1. **Task 1: 扫描并修正 docs/*.md 中遗留的 JSONL 数据源表述** - `5670744` (docs)
2. **Task 2: 生成 07-04 执行摘要（供审计复核）** - (this commit)

## Decisions Made

None - followed plan as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## Next Phase Readiness

- SDK operator docs no longer contain JSONL drift wording and explicitly reference the SQLite metrics source when needed.

---
*Phase: 07-docs-traceability-cleanup*
*Completed: 2026-01-30*
