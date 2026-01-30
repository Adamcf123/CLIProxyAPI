package metricspersist

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPersistenceHealth_DegradedResetsAfterQuietPeriod(t *testing.T) {
	resetPersistenceHealthForTest()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recordPersistenceDrop(DropReasonQueueFull, base)

	within := GetPersistenceHealth(base.Add(1 * time.Minute))
	if within.DroppedTotal != 1 {
		t.Fatalf("DroppedTotal: got=%d want=%d", within.DroppedTotal, 1)
	}
	if !within.Degraded {
		t.Fatalf("Degraded: got=%v want=%v", within.Degraded, true)
	}
	if within.LastDropAt.IsZero() || !within.LastDropAt.Equal(base) {
		t.Fatalf("LastDropAt: got=%s want=%s", within.LastDropAt.Format(time.RFC3339Nano), base.Format(time.RFC3339Nano))
	}
	if within.LastDropReason == nil || *within.LastDropReason != DropReasonQueueFull {
		got := "<nil>"
		if within.LastDropReason != nil {
			got = string(*within.LastDropReason)
		}
		t.Fatalf("LastDropReason: got=%s want=%s", got, DropReasonQueueFull)
	}

	after := GetPersistenceHealth(base.Add(persistenceQuietPeriod + time.Second))
	if after.Degraded {
		t.Fatalf("Degraded after quiet period: got=%v want=%v", after.Degraded, false)
	}
	if after.DroppedTotal != 1 {
		t.Fatalf("DroppedTotal after quiet period: got=%d want=%d", after.DroppedTotal, 1)
	}
}

func TestSQLiteWriter_DropPointsRecordReasons(t *testing.T) {
	resetPersistenceHealthForTest()

	w := newSQLiteWriter(1)

	// 1) writer_not_started: Enqueue before Start
	w.Enqueue(MetricRecord{RequestID: "x"})
	h1 := GetPersistenceHealth(time.Now().UTC())
	if h1.DroppedTotal != 1 {
		t.Fatalf("DroppedTotal after not-started drop: got=%d want=%d", h1.DroppedTotal, 1)
	}
	if h1.LastDropReason == nil || *h1.LastDropReason != DropReasonWriterNotStarted {
		got := "<nil>"
		if h1.LastDropReason != nil {
			got = string(*h1.LastDropReason)
		}
		t.Fatalf("LastDropReason after not-started drop: got=%s want=%s", got, DropReasonWriterNotStarted)
	}

	// 2) queue_full: mark started, fill channel, then Enqueue again
	w.started.Store(true)
	w.queue <- MetricRecord{RequestID: "a"}
	w.Enqueue(MetricRecord{RequestID: "b"})
	h2 := GetPersistenceHealth(time.Now().UTC())
	if h2.DroppedTotal != 2 {
		t.Fatalf("DroppedTotal after queue-full drop: got=%d want=%d", h2.DroppedTotal, 2)
	}
	if h2.LastDropReason == nil || *h2.LastDropReason != DropReasonQueueFull {
		got := "<nil>"
		if h2.LastDropReason != nil {
			got = string(*h2.LastDropReason)
		}
		t.Fatalf("LastDropReason after queue-full drop: got=%s want=%s", got, DropReasonQueueFull)
	}
}

func TestSQLiteWriter_RunPrepareFailureRecordsInsertFailure(t *testing.T) {
	resetPersistenceHealthForTest()

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	// Intentionally skip Migrate so Prepare fails inside run().
	w := newSQLiteWriter(1)
	w.dbMu.Lock()
	w.db = db
	w.dbMu.Unlock()
	w.started.Store(true)
	w.run()

	h := GetPersistenceHealth(time.Now().UTC())
	if h.DroppedTotal != 1 {
		t.Fatalf("DroppedTotal after prepare failure: got=%d want=%d", h.DroppedTotal, 1)
	}
	if h.LastDropReason == nil || *h.LastDropReason != DropReasonInsertFailure {
		got := "<nil>"
		if h.LastDropReason != nil {
			got = string(*h.LastDropReason)
		}
		t.Fatalf("LastDropReason after prepare failure: got=%s want=%s", got, DropReasonInsertFailure)
	}
}

func TestSQLiteWriter_StartPreflightFailsWithoutSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	// No migrations -> Prepare should fail and Start must return an error.
	w := newSQLiteWriter(1)
	if err := w.Start(db); err == nil {
		t.Fatalf("Start: expected error, got nil")
	}
}
