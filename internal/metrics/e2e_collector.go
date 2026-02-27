package metrics

import (
	"sync"
	"time"
)

const (
	// E2EWindowDuration defines the fixed summary window for end-to-end throughput.
	E2EWindowDuration = 10 * time.Minute
	// E2EWindowLabel is the human-readable representation of E2EWindowDuration.
	E2EWindowLabel = "10m"
)

// E2ECollector keeps a sliding window of end-to-end throughput samples
// (computed from output_tokens / total_duration). This is intentionally
// independent from token-timing based metrics which may be suppressed when
// streaming confidence signals are weak.
type E2ECollector struct {
	mu      sync.RWMutex
	windows map[MetricKey]*e2eWindow
	now     func() time.Time
}

type e2eWindow struct {
	mu      sync.RWMutex
	samples []e2eSample
	sumTPS  float64
	sumTPOT float64
}

type e2eSample struct {
	timestamp time.Time
	tps       float64
	tpot      float64
}

func NewE2ECollector() *E2ECollector {
	return newE2ECollectorWithClock(time.Now)
}

func newE2ECollectorWithClock(now func() time.Time) *E2ECollector {
	if now == nil {
		now = time.Now
	}
	return &E2ECollector{windows: make(map[MetricKey]*e2eWindow), now: now}
}

func (c *E2ECollector) Add(key MetricKey, tpsE2E float64, tpotE2E float64) {
	if c == nil {
		return
	}
	if tpsE2E <= 0 || tpotE2E <= 0 {
		return
	}

	w := c.getOrCreateWindow(key)
	w.add(c.now(), tpsE2E, tpotE2E)
}

func (c *E2ECollector) GetAverages(key MetricKey) (count int, tpsAvg float64, tpotAvg float64, ok bool) {
	if c == nil {
		return 0, 0, 0, false
	}
	c.mu.RLock()
	w := c.windows[key]
	c.mu.RUnlock()
	if w == nil {
		return 0, 0, 0, false
	}
	return w.averages(c.now())
}

func (c *E2ECollector) getOrCreateWindow(key MetricKey) *e2eWindow {
	c.mu.RLock()
	w := c.windows[key]
	c.mu.RUnlock()
	if w != nil {
		return w
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	w = c.windows[key]
	if w != nil {
		return w
	}
	w = &e2eWindow{}
	c.windows[key] = w
	return w
}

func (w *e2eWindow) add(now time.Time, tpsE2E float64, tpotE2E float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pruneExpiredLocked(now)
	w.samples = append(w.samples, e2eSample{timestamp: now, tps: tpsE2E, tpot: tpotE2E})
	w.sumTPS += tpsE2E
	w.sumTPOT += tpotE2E
}

func (w *e2eWindow) averages(now time.Time) (count int, tpsAvg float64, tpotAvg float64, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredLocked(now)
	if len(w.samples) <= 0 {
		return 0, 0, 0, false
	}
	den := float64(len(w.samples))
	return len(w.samples), w.sumTPS / den, w.sumTPOT / den, true
}

func (w *e2eWindow) pruneExpiredLocked(now time.Time) {
	if len(w.samples) == 0 {
		return
	}
	cutoff := now.Add(-E2EWindowDuration)
	removeCount := 0
	for removeCount < len(w.samples) {
		sample := w.samples[removeCount]
		if !sample.timestamp.Before(cutoff) {
			break
		}
		w.sumTPS -= sample.tps
		w.sumTPOT -= sample.tpot
		removeCount++
	}
	if removeCount > 0 {
		w.samples = w.samples[removeCount:]
	}
}
