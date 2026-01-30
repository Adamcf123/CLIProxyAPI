package metricspersist

import (
	"path/filepath"
	"testing"
)

func TestCleanup_DeletesRowsOlderThanRetention(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Insert one row older than 7 days and one fresh row.
	if _, err := db.Exec(
		`INSERT INTO metrics (request_id, provider, model, created_at) VALUES (?, ?, ?, datetime('now', '-10 days'));`,
		"old", "openai", "gpt-5.2",
	); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO metrics (request_id, provider, model, created_at) VALUES (?, ?, ?, datetime('now', '-1 days'));`,
		"new", "openai", "gpt-5.2",
	); err != nil {
		t.Fatalf("insert new: %v", err)
	}

	deleted, err := Cleanup(db, 7)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted rows: got=%d want=%d", deleted, 1)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM metrics;`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("remaining rows: got=%d want=%d", count, 1)
	}
}
