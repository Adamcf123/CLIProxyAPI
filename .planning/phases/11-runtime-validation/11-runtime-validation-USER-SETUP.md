# Phase 11: User Setup Required (Runtime Validation)

**Generated:** 2026-02-01
**Phase:** 11-runtime-validation
**Status:** Incomplete

## Environment Variables

| Status | Variable | Source | Add to |
| ------ | -------- | ------ | ------ |
| [ ] | `API_KEY` | Local client API key for validation runs (placeholder ok) | shell env (do not commit) |
| [ ] | `MANAGEMENT_KEY` | Validation-only plaintext management key (suggested: `phase11-dev`) | shell env (do not commit) |

## Local Development Notes

- Baseline runner uses your repo `config.yaml` (derived into artifacts) and expects the configured models to be available.
- Edge-case runner uses a temporary config written under artifacts and a local mock upstream.

## Verification

```bash
bash .planning/phases/11-runtime-validation/scripts/run_baseline.sh --help
bash .planning/phases/11-runtime-validation/scripts/run_edge_cases.sh --help
```
