package metricspersist

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const defaultRetentionDays = 7

// InitDB opens (and creates if missing) a SQLite database file at the given path.
// It also applies required PRAGMAs to stabilize concurrency and performance.
func InitDB(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite db path is required")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Ensure PRAGMA settings consistently apply: SQLite PRAGMAs are connection-scoped.
	// We keep a single underlying connection for now.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := applyPragmas(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite db: %w", err)
	}

	return db, nil
}

func applyPragmas(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA busy_timeout=5000;",
	}

	for _, stmt := range pragmas {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply pragma (%s): %w", stmt, err)
		}
	}

	return nil
}

// Cleanup prunes metrics rows older than retentionDays.
//
// Contract:
// - accuracy: rows are removed based on SQLite's datetime('now', ...) comparison
// - security_boundary: internal-only; retentionDays must be positive
// - side_effects: deletes rows (non-idempotent, but safe to run repeatedly)
func Cleanup(db *sql.DB, retentionDays int) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db is required")
	}
	if retentionDays <= 0 {
		return 0, fmt.Errorf("retentionDays must be > 0")
	}

	// SQLite accepts modifiers like "-7 days" as the second argument to datetime().
	modifier := fmt.Sprintf("-%d days", retentionDays)
	modifier = strings.TrimSpace(modifier)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := db.ExecContext(ctx, `DELETE FROM metrics WHERE created_at < datetime('now', ?);`, modifier)
	if err != nil {
		return 0, fmt.Errorf("delete old metrics: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return rows, nil
}
