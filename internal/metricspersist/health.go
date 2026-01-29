package metricspersist

import (
	"sync/atomic"
	"time"
)

// DropReason is a stable, externally-visible reason code for best-effort
// persistence drops. It is intentionally low-cardinality and MUST NOT include
// request IDs, SQL strings, filesystem paths, or user input.
type DropReason string

const (
	DropReasonQueueFull         DropReason = "queue_full"
	DropReasonWriterNotStarted  DropReason = "writer_not_started"
	DropReasonInsertFailure     DropReason = "insert_failure"
	DropReasonRequestIDConflict DropReason = "request_id_conflict"
)

// PersistenceHealth is a process-lifetime view of best-effort persistence.
//
// Contract:
// - DroppedTotal is monotonically increasing for the process lifetime.
// - Degraded is computed from last_drop_at and a fixed quiet period.
// - LastDropAt is zero when no drop has ever been recorded.
// - LastDropReason is nil when no drop has ever been recorded.
type PersistenceHealth struct {
	Degraded       bool
	DroppedTotal   uint64
	LastDropAt     time.Time
	LastDropReason *DropReason
}

const persistenceQuietPeriod = 5 * time.Minute

type dropReasonCode uint32

const (
	dropReasonNone dropReasonCode = 0

	dropReasonQueueFull         dropReasonCode = 1
	dropReasonWriterNotStarted  dropReasonCode = 2
	dropReasonInsertFailure     dropReasonCode = 3
	dropReasonRequestIDConflict dropReasonCode = 4
)

type persistenceHealthTracker struct {
	droppedTotal      atomic.Uint64
	lastDropUnixNano  atomic.Int64
	lastDropReasonRaw atomic.Uint32
}

var persistenceHealth = &persistenceHealthTracker{}

func dropReasonToCode(r DropReason) dropReasonCode {
	switch r {
	case DropReasonQueueFull:
		return dropReasonQueueFull
	case DropReasonWriterNotStarted:
		return dropReasonWriterNotStarted
	case DropReasonInsertFailure:
		return dropReasonInsertFailure
	case DropReasonRequestIDConflict:
		return dropReasonRequestIDConflict
	default:
		return dropReasonNone
	}
}

func codeToDropReason(c dropReasonCode) (DropReason, bool) {
	switch c {
	case dropReasonQueueFull:
		return DropReasonQueueFull, true
	case dropReasonWriterNotStarted:
		return DropReasonWriterNotStarted, true
	case dropReasonInsertFailure:
		return DropReasonInsertFailure, true
	case dropReasonRequestIDConflict:
		return DropReasonRequestIDConflict, true
	default:
		return "", false
	}
}

// recordPersistenceDrop updates process-level counters and the last-drop marker.
//
// It is designed to be safe and low overhead (no locks) for rare drop paths.
func recordPersistenceDrop(reason DropReason, now time.Time) {
	persistenceHealth.droppedTotal.Add(1)
	persistenceHealth.lastDropUnixNano.Store(now.UTC().UnixNano())
	persistenceHealth.lastDropReasonRaw.Store(uint32(dropReasonToCode(reason)))
}

// GetPersistenceHealth returns the current process-lifetime persistence health.
//
// Degraded automatically returns to false once the quiet period has elapsed
// since the last recorded drop.
func GetPersistenceHealth(now time.Time) PersistenceHealth {
	now = now.UTC()

	total := persistenceHealth.droppedTotal.Load()
	lastUnix := persistenceHealth.lastDropUnixNano.Load()
	reasonCode := dropReasonCode(persistenceHealth.lastDropReasonRaw.Load())

	var out PersistenceHealth
	out.DroppedTotal = total
	if lastUnix > 0 {
		out.LastDropAt = time.Unix(0, lastUnix).UTC()
		if r, ok := codeToDropReason(reasonCode); ok {
			rCopy := r
			out.LastDropReason = &rCopy
		}
		if now.Sub(out.LastDropAt) <= persistenceQuietPeriod {
			out.Degraded = true
		}
	}
	return out
}

func resetPersistenceHealthForTest() {
	persistenceHealth.droppedTotal.Store(0)
	persistenceHealth.lastDropUnixNano.Store(0)
	persistenceHealth.lastDropReasonRaw.Store(0)
}
