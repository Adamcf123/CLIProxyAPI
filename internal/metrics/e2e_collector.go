package metrics

import "sync"

// E2ECollector keeps a sliding window of end-to-end throughput samples
// (computed from output_tokens / total_duration). This is intentionally
// independent from token-timing based metrics which may be suppressed when
// streaming confidence signals are weak.
type E2ECollector struct {
	mu      sync.RWMutex
	windows map[MetricKey]*e2eWindow
}

type e2eWindow struct {
	mu sync.RWMutex

	buf     [windowSize]e2eSample
	head    int
	count   int
	sumTPS  float64
	sumTPOT float64
}

type e2eSample struct {
	tps  float64
	tpot float64
}

func NewE2ECollector() *E2ECollector {
	return &E2ECollector{windows: make(map[MetricKey]*e2eWindow)}
}

func (c *E2ECollector) Add(key MetricKey, tpsE2E float64, tpotE2E float64) {
	if c == nil {
		return
	}
	if tpsE2E <= 0 || tpotE2E <= 0 {
		return
	}

	w := c.getOrCreateWindow(key)
	w.add(tpsE2E, tpotE2E)
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
	return w.averages()
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

func (w *e2eWindow) add(tpsE2E float64, tpotE2E float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.count == windowSize {
		old := w.buf[w.head]
		w.sumTPS -= old.tps
		w.sumTPOT -= old.tpot
	} else {
		w.count++
	}

	w.buf[w.head] = e2eSample{tps: tpsE2E, tpot: tpotE2E}
	w.sumTPS += tpsE2E
	w.sumTPOT += tpotE2E
	w.head = (w.head + 1) % windowSize
}

func (w *e2eWindow) averages() (count int, tpsAvg float64, tpotAvg float64, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.count <= 0 {
		return 0, 0, 0, false
	}
	den := float64(w.count)
	return w.count, w.sumTPS / den, w.sumTPOT / den, true
}
