// Package metrics provides sliding window aggregation and percentile statistics
// for LLM API proxy metrics. It maintains circular buffers of recent requests
// and computes statistical summaries including min, max, avg, median, p95, and p99.
package metrics

import (
	"math"
	"sort"
	"sync"
)

// windowSize is the fixed capacity of the sliding window.
// Each window stores up to 100 recent requests per provider/model combination.
const windowSize = 100

// SlidingWindow maintains a circular buffer of recent RequestMetrics.
// It is safe for concurrent use from multiple goroutines.
// The buffer automatically overwrites the oldest entries when capacity is exceeded.
type SlidingWindow struct {
	// buffer is a fixed-size circular buffer storing request metrics
	buffer [windowSize]RequestMetrics

	// head is the index where the next element will be written
	head int

	// count is the current number of valid elements in the buffer (0 to windowSize)
	count int

	// mu protects concurrent access to buffer, head, and count
	mu sync.RWMutex
}

// NewSlidingWindow creates and initializes a new empty SlidingWindow.
func NewSlidingWindow() *SlidingWindow {
	return &SlidingWindow{}
}

// RestoreFrom restores the window state from a slice of RequestMetrics.
// This is used to load persisted data. The metrics are added in order,
// overwriting any existing data in the window.
func (w *SlidingWindow) RestoreFrom(metrics []RequestMetrics) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Reset window state
	w.head = 0
	w.count = 0

	// Add each metric up to windowSize capacity
	for i, m := range metrics {
		if i >= windowSize {
			break
		}
		w.buffer[w.head] = m
		w.head = (w.head + 1) % windowSize
		w.count++
	}
}

// Add appends a new RequestMetrics to the sliding window.
// If the window is full, this overwrites the oldest entry.
// This method is safe for concurrent use.
func (w *SlidingWindow) Add(m RequestMetrics) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buffer[w.head] = m
	w.head = (w.head + 1) % windowSize
	if w.count < windowSize {
		w.count++
	}
}

// GetAll returns a slice containing all valid elements in the window.
// The returned elements are in insertion order (oldest to newest).
// This method is safe for concurrent use and does not modify the window.
func (w *SlidingWindow) GetAll() []RequestMetrics {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.count == 0 {
		return nil
	}

	// Return a copy to avoid exposing internal buffer state
	result := make([]RequestMetrics, w.count)

	if w.count < windowSize {
		// Buffer not full: elements are from index 0 to count-1
		copy(result, w.buffer[:w.count])
	} else {
		// Buffer full: elements are from head to end, then from 0 to head-1
		copy(result, w.buffer[w.head:])
		copy(result[windowSize-w.head:], w.buffer[:w.head])
	}

	return result
}

// Len returns the current number of valid elements in the window.
// This method is safe for concurrent use.
func (w *SlidingWindow) Len() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.count
}

// GetStats calculates and returns window statistics for TPS, TTFT, and TPOT.
// It computes min, max, avg, median, p95, and p99 for each metric.
// Returns zero WindowStats if the window is empty.
func (w *SlidingWindow) GetStats() WindowStats {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.count == 0 {
		return WindowStats{}
	}

	// Extract all metrics from the window
	metrics := w.getAllUnsafe()

	if len(metrics) == 0 {
		return WindowStats{}
	}

	// Extract individual metric slices
	tpsValues := make([]float64, len(metrics))
	ttftValues := make([]float64, len(metrics))
	tpotValues := make([]float64, len(metrics))

	for i, m := range metrics {
		tpsValues[i] = m.TPS
		ttftValues[i] = m.TTFT
		tpotValues[i] = m.TPOT
	}

	// Calculate statistics
	stats := WindowStats{
		Count: len(metrics),
	}

	stats.TPSMin, stats.TPSMax, stats.TPSAvg, stats.TPSMedian, stats.TPSP95, stats.TPSP99 =
		calculatePercentiles(tpsValues)

	stats.TTFTMin, stats.TTFTMax, stats.TTFTAvg, stats.TTFTMedian, stats.TTFTP95, stats.TTFTP99 =
		calculatePercentiles(ttftValues)

	stats.TPOTMin, stats.TPOTMax, stats.TPOTAvg, stats.TPOTMedian, stats.TPOTP95, stats.TPOTP99 =
		calculatePercentiles(tpotValues)

	return stats
}

// getAllUnsafe returns all valid metrics without locking.
// Caller must hold w.mu.RLock or w.mu.Lock.
func (w *SlidingWindow) getAllUnsafe() []RequestMetrics {
	if w.count == 0 {
		return nil
	}

	// Return a copy to avoid exposing internal buffer state
	result := make([]RequestMetrics, w.count)

	if w.count < windowSize {
		// Buffer not full: elements are from index 0 to count-1
		copy(result, w.buffer[:w.count])
	} else {
		// Buffer full: elements are from head to end, then from 0 to head-1
		copy(result, w.buffer[w.head:])
		copy(result[windowSize-w.head:], w.buffer[:w.head])
	}

	return result
}

// calculatePercentiles computes min, max, avg, median, p95, and p99 from a float64 slice
func calculatePercentiles(values []float64) (min, max, avg, median, p95, p99 float64) {
	if len(values) == 0 {
		return 0, 0, 0, 0, 0, 0
	}

	// Sort for percentile calculations
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	min = sorted[0]
	max = sorted[len(sorted)-1]

	// Calculate average
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	avg = sum / float64(len(sorted))

	// Calculate percentiles using linear interpolation
	median = calculatePercentile(sorted, 50)
	p95 = calculatePercentile(sorted, 95)
	p99 = calculatePercentile(sorted, 99)

	return min, max, avg, median, p95, p99
}

// RestoreFromMetrics restores the window state from a slice of RequestMetrics.
// This is used for loading persisted data after process restarts.
// The metrics are added to the window in order, with the oldest being at index 0.
// If the number of metrics exceeds windowSize, only the most recent windowSize are kept.
func (w *SlidingWindow) RestoreFromMetrics(metrics []RequestMetrics) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Reset window state
	w.count = 0
	w.head = 0

	// Add each metric to the window
	for _, m := range metrics {
		w.buffer[w.head] = m
		w.head = (w.head + 1) % windowSize
		if w.count < windowSize {
			w.count++
		}
	}
}

// calculateStats computes WindowStats from a slice of RequestMetrics.
// It calculates min, max, avg, median, p95, and p99 for TPS, TTFT, and TPOT.
func calculateStats(key MetricKey, metrics []RequestMetrics) WindowStats {
	if len(metrics) == 0 {
		return WindowStats{Key: key}
	}

	// Extract individual metric slices
	tpsValues := make([]float64, len(metrics))
	ttftValues := make([]float64, len(metrics))
	tpotValues := make([]float64, len(metrics))

	for i, m := range metrics {
		tpsValues[i] = m.TPS
		ttftValues[i] = m.TTFT
		tpotValues[i] = m.TPOT
	}

	// Calculate statistics
	stats := WindowStats{
		Key:   key,
		Count: len(metrics),
	}

	stats.TPSMin, stats.TPSMax, stats.TPSAvg, stats.TPSMedian, stats.TPSP95, stats.TPSP99 =
		calculatePercentiles(tpsValues)

	stats.TTFTMin, stats.TTFTMax, stats.TTFTAvg, stats.TTFTMedian, stats.TTFTP95, stats.TTFTP99 =
		calculatePercentiles(ttftValues)

	stats.TPOTMin, stats.TPOTMax, stats.TPOTAvg, stats.TPOTMedian, stats.TPOTP95, stats.TPOTP99 =
		calculatePercentiles(tpotValues)

	return stats
}

// calculatePercentile computes the p-th percentile from a sorted slice of float64 values.
// p is in the range [0, 100].
// The function uses linear interpolation for non-integer indices.
//
// Formula: index = (p/100) * (n-1)
// If index is an integer, return that element.
// Otherwise, interpolate between the floor and ceiling elements.
func calculatePercentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	index := (p / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sorted[lower]
	}

	// Linear interpolation between sorted[lower] and sorted[upper]
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
