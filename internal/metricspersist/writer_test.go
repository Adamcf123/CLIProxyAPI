package metricspersist

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

type persistenceHealthSnapshot struct {
	droppedTotal     uint64
	lastDropUnixNano int64
	lastReasonRaw    uint32
}

func snapshotPersistenceHealth() persistenceHealthSnapshot {
	return persistenceHealthSnapshot{
		droppedTotal:     persistenceHealth.droppedTotal.Load(),
		lastDropUnixNano: persistenceHealth.lastDropUnixNano.Load(),
		lastReasonRaw:    persistenceHealth.lastDropReasonRaw.Load(),
	}
}

func restorePersistenceHealth(s persistenceHealthSnapshot) {
	persistenceHealth.droppedTotal.Store(s.droppedTotal)
	persistenceHealth.lastDropUnixNano.Store(s.lastDropUnixNano)
	persistenceHealth.lastDropReasonRaw.Store(s.lastReasonRaw)
}

func TestAsyncWriter_PersistsRowsAndDedupesByRequestID(t *testing.T) {
	snap := snapshotPersistenceHealth()
	t.Cleanup(func() { restorePersistenceHealth(snap) })

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

	w := newSQLiteWriter(writerQueueSize)
	if err := w.Start(db); err != nil {
		t.Fatalf("writer.Start: %v", err)
	}

	provider := "openai"
	model := "gpt-5.2"

	boolPtr := func(v bool) *bool { return &v }

	// Enqueue multiple distinct records plus one duplicate request_id.
	w.Enqueue(MetricRecord{RequestID: "r1", Provider: provider, Model: model, Streaming: boolPtr(true)})
	w.Enqueue(MetricRecord{RequestID: "r2", Provider: provider, Model: model, Streaming: boolPtr(false)})
	w.Enqueue(MetricRecord{RequestID: "r3", Provider: provider, Model: model})
	w.Enqueue(MetricRecord{RequestID: "r2", Provider: provider, Model: model}) // duplicate

	waitForCount(t, db, 3)

	// Ensure streaming is persisted as INTEGER 0/1.
	if got := queryStreaming(t, db, "r1"); got != 1 {
		t.Fatalf("streaming for r1: got=%d want=1", got)
	}
	if got := queryStreaming(t, db, "r2"); got != 0 {
		t.Fatalf("streaming for r2: got=%d want=0", got)
	}
}

func TestPersistenceHealth_WriterNotStartedDropIsObservable(t *testing.T) {
	snapHealth := snapshotPersistenceHealth()
	prevWriter := defaultWriter
	t.Cleanup(func() {
		defaultWriter = prevWriter
		restorePersistenceHealth(snapHealth)
	})

	defaultWriter = newSQLiteWriter(writerQueueSize)

	now := time.Now().UTC()
	baseline := GetPersistenceHealth(now)

	Enqueue(MetricRecord{RequestID: "r-writer-not-started", Provider: "openai", Model: "gpt-5.2"})

	h := GetPersistenceHealth(time.Now().UTC())
	if h.DroppedTotal != baseline.DroppedTotal+1 {
		t.Fatalf("DroppedTotal: got=%d want=%d", h.DroppedTotal, baseline.DroppedTotal+1)
	}
	if h.LastDropReason == nil || *h.LastDropReason != DropReasonWriterNotStarted {
		t.Fatalf("LastDropReason: got=%v want=%q", h.LastDropReason, DropReasonWriterNotStarted)
	}
	if !h.Degraded {
		t.Fatalf("Degraded: got=false want=true")
	}
}

func TestSQLiteWriter_StartFailsPreflightWithoutSchema(t *testing.T) {
	snap := snapshotPersistenceHealth()
	t.Cleanup(func() { restorePersistenceHealth(snap) })

	dbPath := filepath.Join(t.TempDir(), "no-migrations.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	w := newSQLiteWriter(1)
	if err := w.Start(db); err == nil {
		t.Fatalf("writer.Start: expected error when schema is missing")
	}
}

func TestPersistenceHealth_QueueFullDropIsObservable(t *testing.T) {
	snapHealth := snapshotPersistenceHealth()
	prevWriter := defaultWriter
	t.Cleanup(func() {
		defaultWriter = prevWriter
		restorePersistenceHealth(snapHealth)
	})

	w := newSQLiteWriter(1)
	w.started.Store(true) // mark started but never run; queue will fill deterministically
	defaultWriter = w

	now := time.Now().UTC()
	baseline := GetPersistenceHealth(now)

	Enqueue(MetricRecord{RequestID: "r-q1", Provider: "openai", Model: "gpt-5.2"})
	Enqueue(MetricRecord{RequestID: "r-q2", Provider: "openai", Model: "gpt-5.2"})

	h := GetPersistenceHealth(time.Now().UTC())
	if h.DroppedTotal != baseline.DroppedTotal+1 {
		t.Fatalf("DroppedTotal: got=%d want=%d", h.DroppedTotal, baseline.DroppedTotal+1)
	}
	if h.LastDropReason == nil || *h.LastDropReason != DropReasonQueueFull {
		t.Fatalf("LastDropReason: got=%v want=%q", h.LastDropReason, DropReasonQueueFull)
	}
	if !h.Degraded {
		t.Fatalf("Degraded: got=false want=true")
	}
}

func TestPersistenceHealth_InsertFailureIsObservable(t *testing.T) {
	snap := snapshotPersistenceHealth()
	t.Cleanup(func() { restorePersistenceHealth(snap) })

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	if err := Migrate(db); err != nil {
		_ = db.Close()
		t.Fatalf("Migrate: %v", err)
	}

	w := newSQLiteWriter(writerQueueSize)
	if err := w.Start(db); err != nil {
		_ = db.Close()
		t.Fatalf("writer.Start: %v", err)
	}

	baseline := GetPersistenceHealth(time.Now().UTC()).DroppedTotal

	// Force a deterministic insert error by closing the DB after startup.
	_ = db.Close()

	w.Enqueue(MetricRecord{RequestID: "r-insert-failure", Provider: "openai", Model: "gpt-5.2"})

	deadline := time.Now().Add(2 * time.Second)
	for {
		h := GetPersistenceHealth(time.Now().UTC())
		if h.DroppedTotal >= baseline+1 && h.LastDropReason != nil && *h.LastDropReason == DropReasonInsertFailure {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for insert failure: DroppedTotal=%d LastDropReason=%v", h.DroppedTotal, h.LastDropReason)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPersistenceHealth_QuietPeriodClearsDegraded(t *testing.T) {
	snap := snapshotPersistenceHealth()
	t.Cleanup(func() { restorePersistenceHealth(snap) })

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	baseline := GetPersistenceHealth(baseTime)

	recordPersistenceDrop(DropReasonQueueFull, baseTime)

	during := GetPersistenceHealth(baseTime.Add(1 * time.Second))
	if during.DroppedTotal != baseline.DroppedTotal+1 {
		t.Fatalf("DroppedTotal: got=%d want=%d", during.DroppedTotal, baseline.DroppedTotal+1)
	}
	if !during.Degraded {
		t.Fatalf("Degraded during quiet period: got=false want=true")
	}
	if during.LastDropAt.IsZero() {
		t.Fatalf("LastDropAt: expected non-zero")
	}

	after := GetPersistenceHealth(baseTime.Add(persistenceQuietPeriod + 1*time.Second))
	if after.DroppedTotal != during.DroppedTotal {
		t.Fatalf("DroppedTotal after quiet period: got=%d want=%d", after.DroppedTotal, during.DroppedTotal)
	}
	if after.Degraded {
		t.Fatalf("Degraded after quiet period: got=true want=false")
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
