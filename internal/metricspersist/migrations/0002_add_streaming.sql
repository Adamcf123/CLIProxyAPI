-- +goose Up
-- Add streaming dimension for Query API filtering/grouping.
-- Historical rows cannot be reliably backfilled; default to 0 to avoid NULL group keys.
ALTER TABLE metrics ADD COLUMN streaming INTEGER NOT NULL DEFAULT 0;

-- Indexes to support time-range scans and provider/model/streaming grouping.
CREATE INDEX IF NOT EXISTS idx_metrics_created_at ON metrics(created_at);
CREATE INDEX IF NOT EXISTS idx_metrics_dim_time ON metrics(provider, model, streaming, created_at);

-- +goose Down
-- SQLite does not reliably support dropping columns across versions.
-- Be conservative: remove only the indexes created in this migration.
DROP INDEX IF EXISTS idx_metrics_dim_time;
DROP INDEX IF EXISTS idx_metrics_created_at;
