---
phase: 07-docs-traceability-cleanup
plan: 03
subsystem: docs
tags: [sqlite, metrics, management-api, audit, traceability]

# Dependency graph
requires:
  - phase: 03-persistence
    provides: SQLite metrics persistence at logs/metrics.db
  - phase: 04-query-api
    provides: GET /v0/management/metrics query endpoint
provides:
  - planning/operator docs consistently describe SQLite (logs/metrics.db) as the single metrics source
  - removal of legacy JSONL wording to prevent operator confusion during audits
affects: [07-docs-traceability-cleanup, audit, operations]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - docs drift cleanup guarded by executable text checks (python snippets)

key-files:
  created:
    - .planning/phases/07-docs-traceability-cleanup/07-03-SUMMARY.md
  modified:
    - .planning/STATE.md
    - 重要命令.txt

key-decisions:
  - "None - followed plan as specified"

patterns-established:
  - "Docs: metrics persistence source-of-truth is SQLite logs/metrics.db"

# Metrics
duration: 2 min
completed: 2026-01-30
---

# Phase 7 Plan 03: Docs Drift Cleanup Summary

**Planning/operator docs now consistently point to SQLite `logs/metrics.db` as the metrics source, with `GET /v0/management/metrics` as the query entrypoint.**

## Performance

- **Duration:** 2 min
- **Started:** 2026-01-30T16:44:25Z
- **Completed:** 2026-01-30T16:46:34Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments

- Removed JSONL persistence wording from planning docs so operators are not misled into looking for legacy files.
- Updated operator command snippets to reference the SQLite single source of truth (`logs/metrics.db`) and the management query endpoint.
- Added an auditable, executable verification trail (python checks + ripgrep checks) to prevent drift reappearing.

## What Changed (by file)

### `.planning/STATE.md`

- Rewrote the Phase 02 "metrics persistence" wording to match current implementation: `MetricsPlugin` persists asynchronously to SQLite (default `logs/metrics.db`).
- Removed all `jsonl` mentions to eliminate conflicting statements across phases.

### `重要命令.txt`

- Ensured operator-facing guidance no longer implies JSONL as a data source.
- Added an example `GET /v0/management/metrics` query snippet and kept `logs/metrics.db` reference.

## Evidence (implementation facts)

- SQLite file path is explicitly the default in server startup (`cmd/server/main.go`):
  - `const metricsDBPath = "logs/metrics.db"`
  - `metricspersist.InitDB(metricsDBPath)` + `metricspersist.Migrate(db)` + `metricspersist.StartWriter(db)`
- Management query handler uses `logs/metrics.db` as the default read DB path (`internal/api/handlers/management/handler.go`):
  - `metricsDBPath: "logs/metrics.db"`
  - `openMetricsReadDB()` falls back to `logs/metrics.db` when unset
- The public management query endpoint path is `/v0/management/metrics` (validated by tests):
  - `test/metrics_management_test.go` constructs requests like `"/v0/management/metrics?mode=percentiles"`

## Verify

Task 1 verification (STATE.md drift cleanup):

```bash
python - <<'PY'
import pathlib
import sys

path = pathlib.Path('.planning/STATE.md')
text = path.read_text(encoding='utf-8')
low = text.lower()

if 'logs/metrics.db' not in text:
    print('STATE.md must mention logs/metrics.db')
    sys.exit(1)

if '.jsonl' in low or 'jsonl' in low:
    print('STATE.md should not mention jsonl after cleanup')
    sys.exit(1)

print('ok')
PY
```

Task 2 verification (operator docs cleanup):

```bash
python - <<'PY'
import pathlib
import sys

needles = ['.jsonl', 'jsonl', 'metrics-yyyy']

cmds = pathlib.Path('重要命令.txt')
if not cmds.exists():
    print('重要命令.txt missing')
    sys.exit(1)

text = cmds.read_text(encoding='utf-8').lower()
for n in needles:
    if n in text:
        print(f"unexpected '{n}' mention in {cmds}")
        sys.exit(1)

if 'metrics.db' not in text:
    print('重要命令.txt should mention metrics.db')
    sys.exit(1)

print('ok')
PY
```

Task 3 verification (summary contains required audit hooks):

```bash
rg -nS "Phase 7" .planning/phases/07-docs-traceability-cleanup/07-03-SUMMARY.md
rg -nS "STATE\\.md" .planning/phases/07-docs-traceability-cleanup/07-03-SUMMARY.md
rg -nS "logs/metrics\\.db|v0/management/metrics" .planning/phases/07-docs-traceability-cleanup/07-03-SUMMARY.md
```

## Task Commits

Each task was committed atomically:

1. **Task 1: 修复 STATE.md 中关于指标落盘位置的事实漂移** - `1295fc3` (docs)
2. **Task 2: 清理 重要命令.txt 中遗留的 JSONL 数据源表述** - `fa198f1` (docs)
3. **Task 3: 生成 07-03 执行摘要（供审计复核）** - (this commit)

## Decisions Made

None - followed plan as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- `internal/api/handlers/management/handler.go` and `cmd/server/main.go` clearly establish the SQLite path (`logs/metrics.db`), but the literal string `"/v0/management/metrics"` is not present in those two files; the endpoint path is evidenced via `test/metrics_management_test.go`.

## Next Phase Readiness

- Docs drift about JSONL vs SQLite is removed for the touched planning/operator docs.
- Ready for remaining Phase 7 cleanup plans.

---
*Phase: 07-docs-traceability-cleanup*
*Completed: 2026-01-30*
