package metricspersist

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	writerQueueSize = 1024
	insertTimeout   = 2 * time.Second
)

const metricsInsertSQL = `INSERT INTO metrics (
	request_id,
	provider,
	model,
	streaming,
	tps,
	ttft,
	tpot,
	input_tokens,
	output_tokens,
	total_tokens,
	duration_ms,
	status_code,
	error_info
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(request_id) DO NOTHING;`

var defaultWriter = newSQLiteWriter(writerQueueSize)

// StartWriter starts the background worker that persists enqueued MetricRecord
// values into the provided SQLite database.
func StartWriter(db *sql.DB) error {
	return defaultWriter.Start(db)
}

// Enqueue attempts to enqueue a metrics record for writing.
// It never blocks the caller; when the internal queue is full (or the writer
// hasn't been started), the record is dropped.
func Enqueue(record MetricRecord) {
	defaultWriter.Enqueue(record)
}

type sqliteWriter struct {
	queue chan MetricRecord

	startOnce sync.Once
	started   atomic.Bool

	dbMu sync.Mutex
	db   *sql.DB
}

func newSQLiteWriter(queueSize int) *sqliteWriter {
	if queueSize <= 0 {
		queueSize = writerQueueSize
	}
	return &sqliteWriter{queue: make(chan MetricRecord, queueSize)}
}

func (w *sqliteWriter) Start(db *sql.DB) error {
	if w == nil {
		return nil
	}
	if db == nil {
		return fmt.Errorf("db is required")
	}

	// Fail-fast preflight: the writer goroutine must not fail-silent on Prepare.
	// This keeps startup semantics explicit (server startup can os.Exit(1)).
	if err := preflightPrepareMetricsInsert(db); err != nil {
		return err
	}

	w.dbMu.Lock()
	if w.db == nil {
		w.db = db
	}
	w.dbMu.Unlock()

	// Start is intended to be called once during server startup.
	w.startOnce.Do(func() {
		w.started.Store(true)
		go w.run()
	})

	return nil
}

func (w *sqliteWriter) Enqueue(record MetricRecord) {
	if w == nil {
		return
	}
	if !w.started.Load() {
		// Writer not started yet; drop to keep request path unaffected.
		recordPersistenceDrop(DropReasonWriterNotStarted, time.Now().UTC())
		return
	}
	select {
	case w.queue <- record:
	default:
		// Drop on queue full to ensure this stays off the main request path.
		recordPersistenceDrop(DropReasonQueueFull, time.Now().UTC())
	}
}

func preflightPrepareMetricsInsert(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	stmt, err := db.Prepare(metricsInsertSQL)
	if err != nil {
		return fmt.Errorf("prepare metrics insert: %w", err)
	}
	_ = stmt.Close()
	return nil
}

func (w *sqliteWriter) run() {
	// Snapshot DB pointer once; it is expected to remain valid for the process lifetime.
	w.dbMu.Lock()
	db := w.db
	w.dbMu.Unlock()
	if db == nil {
		return
	}

	// Keep the DB self-cleaning even for long-lived processes.
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer cleanupTicker.Stop()

	stmt, err := db.Prepare(metricsInsertSQL)
	if err != nil {
		// Best-effort: do not block requests, but do not fail silent.
		recordPersistenceDrop(DropReasonInsertFailure, time.Now().UTC())
		return
	}
	defer func() { _ = stmt.Close() }()

	insert := func(r MetricRecord) {
		if r.RequestID == "" || r.Provider == "" || r.Model == "" {
			return
		}
		streaming := int64(0)
		if r.Streaming != nil && *r.Streaming {
			streaming = 1
		}
		ctx, cancel := context.WithTimeout(context.Background(), insertTimeout)
		defer cancel()
		_, err := stmt.ExecContext(
			ctx,
			r.RequestID,
			r.Provider,
			r.Model,
			streaming,
			r.TPS,
			r.TTFT,
			r.TPOT,
			r.InputTokens,
			r.OutputTokens,
			r.TotalTokens,
			r.DurationMS,
			r.StatusCode,
			r.ErrorInfo,
		)
		if err != nil {
			recordPersistenceDrop(DropReasonInsertFailure, time.Now().UTC())
		}
	}

	// Best-effort: retention should never impact request path.
	_, _ = Cleanup(db, defaultRetentionDays)

	for {
		select {
		case r, ok := <-w.queue:
			if !ok {
				return
			}
			insert(r)
		case <-cleanupTicker.C:
			_, _ = Cleanup(db, defaultRetentionDays)
		}
	}
}
