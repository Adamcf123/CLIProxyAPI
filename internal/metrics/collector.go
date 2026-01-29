package metrics

import (
	"math"
	"sort"
	"sync"
	"time"
)

// windowSize is the fixed size of the sliding window (number of requests)
const windowSize = 100

// Persistence defines the interface for persisting and loading sliding window data.
// This allows windows to be restored after process restarts.
// Implementations can be nil if persistence is not required.
type Persistence interface {
	// Save persists the current window state for a given key
	Save(key MetricKey, window []RequestMetrics) error
	// Load retrieves the previously persisted window state for a given key
	Load(key MetricKey) ([]RequestMetrics, error)
}

// TPSCollector manages the collection and aggregation of TPS metrics.
// It maintains separate sliding windows for each MetricKey (provider/model/streaming combination).
type TPSCollector struct {
	// windows holds the sliding window for each grouping key
	windows map[MetricKey]*SlidingWindow

	// mu protects concurrent access to windows map
	mu sync.RWMutex

	// persistence is the optional persistence layer for window data
	persistence Persistence
}

// SlidingWindow implements a fixed-size circular buffer for request metrics.
// It maintains the most recent windowSize RequestMetrics records.
type SlidingWindow struct {
	// buffer is the circular buffer storing request metrics
	buffer [windowSize]RequestMetrics

	// pos is the current write position in the circular buffer
	pos int

	// count is the number of elements currently in the buffer (0 to windowSize)
	count int
}

// NewCollector creates a new TPSCollector instance.
// The persistence parameter is optional; pass nil if persistence is not required.
func NewCollector(persistence Persistence) *TPSCollector {
	return &TPSCollector{
		windows:     make(map[MetricKey]*SlidingWindow),
		persistence: persistence,
	}
}

// StartRequest begins tracking a new request for the given metric key.
// It returns a RequestMetrics object that should be used for subsequent
// RecordFirstToken and CompleteRequest calls.
func (c *TPSCollector) StartRequest(key MetricKey) *RequestMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get or create window for this key
	window, exists := c.windows[key]
	if !exists {
		window = &SlidingWindow{}
		c.windows[key] = window

		// Try to load persisted data if persistence is configured
		if c.persistence != nil {
			if loaded, err := c.persistence.Load(key); err == nil && len(loaded) > 0 {
				// Restore window state from persisted data
				for i, m := range loaded {
					if i < windowSize {
						window.buffer[i] = m
						window.count = i + 1
						window.pos = (i + 1) % windowSize
					}
				}
			}
		}
	}

	// Create new metrics entry
	metrics := &RequestMetrics{
		Key:       key,
		StartTime: time.Now(),
	}

	// Reserve slot in buffer (will be overwritten during CompleteRequest)
	window.pos = (window.pos + 1) % windowSize
	if window.count < windowSize {
		window.count++
	}

	return metrics
}

// RecordFirstToken records the time when the first output token is received.
// This is used to calculate TTFT (Time To First Token).
func (c *TPSCollector) RecordFirstToken(m *RequestMetrics) {
	if m == nil {
		return
	}
	m.FirstTokenTime = time.Now()
}

// CompleteRequest finalizes a request's metrics and calculates TPS, TTFT, and TPOT.
// The metrics are stored in the sliding window for aggregation.
//
// Calculations:
//   - TTFT = FirstTokenTime - StartTime (seconds)
//   - TPOT = (EndTime - FirstTokenTime) / OutputTokens (seconds per token)
//   - TPS = OutputTokens / (EndTime - FirstTokenTime) (tokens per second)
//
// If outputTokens is 0, the request is discarded (no metrics recorded).
func (c *TPSCollector) CompleteRequest(m *RequestMetrics, outputTokens int) {
	if m == nil || outputTokens <= 0 {
		// Discard requests with no output tokens
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Calculate timing metrics
	now := time.Now()
	m.EndTime = now
	m.OutputTokens = outputTokens

	// Calculate durations
	tokenGenerationDuration := m.EndTime.Sub(m.FirstTokenTime).Seconds()
	m.TTFT = m.FirstTokenTime.Sub(m.StartTime).Seconds()

	// Calculate TPOT (Time Per Output Token)
	if outputTokens > 0 && tokenGenerationDuration > 0 {
		m.TPOT = tokenGenerationDuration / float64(outputTokens)
		m.TPS = float64(outputTokens) / tokenGenerationDuration
		// Round TPS to 2 decimal places
		m.TPS = math.Round(m.TPS*100) / 100
	}

	// Store in window
	window := c.windows[m.Key]
	if window == nil {
		// This shouldn't happen as StartRequest creates the window
		window = &SlidingWindow{}
		c.windows[m.Key] = window
	}

	// Write to current position (reserved in StartRequest)
	window.buffer[window.pos] = *m

	// Persist if configured
	if c.persistence != nil {
		metrics := window.getMetrics()
		_ = c.persistence.Save(m.Key, metrics) // Ignore persistence errors
	}
}

// GetWindowStats returns aggregated statistics for the given metric key.
// It calculates min, max, avg, median, p95, and p99 for TPS, TTFT, and TPOT.
func (c *TPSCollector) GetWindowStats(key MetricKey) WindowStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	window := c.windows[key]
	if window == nil || window.count == 0 {
		return WindowStats{Key: key}
	}

	metrics := window.getMetrics()

	return calculateStats(key, metrics)
}

// getMetrics returns all valid metrics from the window buffer
func (w *SlidingWindow) getMetrics() []RequestMetrics {
	if w.count == 0 {
		return nil
	}

	// If buffer is not yet full, return elements 0 to count
	if w.count < windowSize {
		return w.buffer[:w.count]
	}

	// Buffer is full: return elements from pos+1 to end, then 0 to pos
	result := make([]RequestMetrics, windowSize)
	copy(result, w.buffer[w.pos+1:])
	copy(result[windowSize-w.pos-1:], w.buffer[:w.pos+1])
	return result
}

// calculateStats computes statistics from a slice of RequestMetrics
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
	median = percentile(sorted, 50)
	p95 = percentile(sorted, 95)
	p99 = percentile(sorted, 99)

	return min, max, avg, median, p95, p99
}

// percentile calculates the p-th percentile using linear interpolation
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	index := (float64(p) / 100.0) * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[lower+1]*weight
}
