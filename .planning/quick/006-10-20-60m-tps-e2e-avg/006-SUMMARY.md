---
phase: quick-006-10-20-60m-tps-e2e-avg
plan: 006
subsystem: metrics
tags: [metrics, buckets, tps, sqlite, management-api]

provides:
  - "Buckets mode adds end-to-end throughput average (tps_e2e_avg) and its sample count (tps_e2e_sample_count)"
  - "tps_e2e_avg is computed only from usable samples: output_tokens != NULL and duration_ms > 0 (canceled excluded)"

key_files:
  modified:
    - internal/api/handlers/management/metrics.go
    - test/metrics_management_test.go

completed: 2026-02-02
---

# Quick Task 006: Buckets tps_e2e_avg

**在 `/v0/management/metrics?mode=buckets` 的每个 bucket 里新增端到端吞吐平均值（tps_e2e_avg）与样本数（tps_e2e_sample_count），用于窗口汇总时直接读取端到端吞吐。**

## What Changed

- buckets 聚合输出（`internal/api/handlers/management/metrics.go`）新增：
  - `metrics.tps_e2e_avg`：`output_tokens / (duration_ms/1000)` 的 bucket 平均值
  - `metrics.tps_e2e_sample_count`：满足 `output_tokens IS NOT NULL AND duration_ms > 0` 的样本数
- 口径与 buckets 现有语义保持一致：
  - canceled（status_code=499）继续在主 buckets 聚合中被排除，不污染 success/failure 的平均值与 count
  - 当 bucket 内无可用样本时：`tps_e2e_sample_count=0` 且 `tps_e2e_avg=null`

## Tests

- `go test ./...`

## Task Commits

- Task 1: buckets SQL 聚合新增 tps_e2e_avg + sample_count
  - `0f70a8e` feat(quick-006-10-20-60m-tps-e2e-avg): add buckets tps_e2e_avg metric
- Task 2: 扩展 buckets mode 测试覆盖新字段契约
  - `398de6b` test(quick-006-10-20-60m-tps-e2e-avg): assert buckets tps_e2e JSON contract

## Execution

- Started: 2026-02-02T08:34:32Z
- Completed: 2026-02-02T08:40:50Z
- Duration: 6 min
