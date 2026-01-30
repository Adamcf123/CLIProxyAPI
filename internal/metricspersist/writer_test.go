package metricspersist

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestAsyncWriter_PersistsRowsAndDedupesByRequestID(t *testing.T) {
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
	if err := StartWriter(db); err != nil {
		t.Fatalf("StartWriter: %v", err)
	}

	provider := "openai"
	model := "gpt-5.2"

	boolPtr := func(v bool) *bool { return &v }

	// Enqueue multiple distinct records plus one duplicate request_id.
	Enqueue(MetricRecord{RequestID: "r1", Provider: provider, Model: model, Streaming: boolPtr(true)})
	Enqueue(MetricRecord{RequestID: "r2", Provider: provider, Model: model, Streaming: boolPtr(false)})
	Enqueue(MetricRecord{RequestID: "r3", Provider: provider, Model: model})
	Enqueue(MetricRecord{RequestID: "r2", Provider: provider, Model: model}) // duplicate

	waitForCount(t, db, 3)

	// Ensure streaming is persisted as INTEGER 0/1.
	if got := queryStreaming(t, db, "r1"); got != 1 {
		t.Fatalf("streaming for r1: got=%d want=1", got)
	}
	if got := queryStreaming(t, db, "r2"); got != 0 {
		t.Fatalf("streaming for r2: got=%d want=0", got)
	}
}

func queryStreaming(t *testing.T, db *sql.DB, requestID string) int64 {
	t.Helper()
	var got int64
	if err := db.QueryRow("SELECT streaming FROM metrics WHERE request_id = ?;", requestID).Scan(&got); err != nil {
		t.Fatalf("query streaming: %v", err)
	}
	return got
}

func waitForCount(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var got int
		if err := db.QueryRow("SELECT COUNT(*) FROM metrics;").Scan(&got); err != nil {
			t.Fatalf("query count: %v", err)
		}
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for row count: got=%d want=%d", got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
