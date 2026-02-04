-- +goose Up
-- Rename the stored TPS metric to make semantics explicit (TPS during generation window).
-- User requested to drop historical data; this migration recreates the table.

DROP INDEX IF EXISTS idx_metrics_dim_time;
DROP INDEX IF EXISTS idx_metrics_created_at;

DROP TABLE IF EXISTS metrics;

CREATE TABLE IF NOT EXISTS metrics (
  request_id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  streaming INTEGER NOT NULL DEFAULT 0,
  tps_gen REAL,
  ttft REAL,
  tpot REAL,
  input_tokens INTEGER,
  output_tokens INTEGER,
  total_tokens INTEGER,
  duration_ms INTEGER,
  status_code INTEGER,
  error_info TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes to support time-range scans and provider/model/streaming grouping.
CREATE INDEX IF NOT EXISTS idx_metrics_created_at ON metrics(created_at);
CREATE INDEX IF NOT EXISTS idx_metrics_dim_time ON metrics(provider, model, streaming, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_metrics_dim_time;
DROP INDEX IF EXISTS idx_metrics_created_at;

DROP TABLE IF EXISTS metrics;

-- Recreate the pre-rename schema (data is not preserved).
CREATE TABLE IF NOT EXISTS metrics (
  request_id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  streaming INTEGER NOT NULL DEFAULT 0,
  tps REAL,
  ttft REAL,
  tpot REAL,
  input_tokens INTEGER,
  output_tokens INTEGER,
  total_tokens INTEGER,
  duration_ms INTEGER,
  status_code INTEGER,
  error_info TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_metrics_created_at ON metrics(created_at);
CREATE INDEX IF NOT EXISTS idx_metrics_dim_time ON metrics(provider, model, streaming, created_at);
