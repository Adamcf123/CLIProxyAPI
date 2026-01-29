-- +goose Up
CREATE TABLE IF NOT EXISTS metrics (
  request_id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
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

-- +goose Down
DROP TABLE IF EXISTS metrics;
