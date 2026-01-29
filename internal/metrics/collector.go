// Package metrics provides TPS (Tokens Per Second) calculation and aggregation
// for LLM API proxy requests. It tracks TPS, TTFT (Time To First Token),
// and TPOT (Time Per Output Token) metrics grouped by provider, model, and streaming mode.
package metrics

import (
	"time"
)

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

	// persistence is the optional persistence layer for window data
	persistence Persistence
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
	// Ensure window exists for this key (created lazily on first access)
	_ = c.getOrCreateWindow(key)

	// Create new metrics entry
	metrics := &RequestMetrics{
		Key:       key,
		StartTime: time.Now(),
	}

	return metrics
}

// RecordFirstToken records the time when the first output token is received.
// This is used to calculate TTFT (Time To First Token).
// For non-streaming requests, this method should NOT be called.
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
//   - For streaming requests:
//     * TTFT = FirstTokenTime - StartTime (seconds)
//     * TPOT = (EndTime - FirstTokenTime) / OutputTokens (seconds per token)
//     * TPS = OutputTokens / (EndTime - FirstTokenTime) (tokens per second)
//   - For non-streaming requests:
//     * TTFT = EndTime - StartTime (total response time, seconds)
//     * TPOT = TTFT / OutputTokens (average time per token)
//     * TPS = OutputTokens / TTFT (tokens per second)
//
// Returns error if:
//   - metrics is nil
//   - outputTokens is 0 (request is discarded, not recorded)
//   - calculation fails (invalid timing data)
//
// If an error is returned, the request is NOT recorded in the window.
func (c *TPSCollector) CompleteRequest(m *RequestMetrics, outputTokens int) error {
	if m == nil {
		return ErrNilMetrics
	}

	if outputTokens <= 0 {
		return ErrNonPositiveTokens
	}

	// Set end time
	m.EndTime = time.Now()
	m.OutputTokens = outputTokens

	// Calculate metrics based on streaming mode
	var err error
	if m.Key.Streaming {
		// Streaming: use FirstTokenTime for TTFT and TPS/TPOT calculations
		m.TTFT, err = CalculateTTFT(m.StartTime, m.FirstTokenTime)
		if err != nil {
			return err
		}

		m.TPS, err = CalculateTPS(outputTokens, m.FirstTokenTime, m.EndTime)
		if err != nil {
			return err
		}

		m.TPOT, err = CalculateTPOT(outputTokens, m.FirstTokenTime, m.EndTime)
		if err != nil {
			return err
		}
	} else {
		// Non-streaming: TTFT = total response time, use StartTime for TPS/TPOT
		totalTime := m.EndTime.Sub(m.StartTime)
		if totalTime <= 0 {
			return ErrNonPositiveGenerationTime
		}

		m.TTFT = totalTime.Seconds()
		m.TPOT = m.TTFT / float64(outputTokens)

		// Calculate TPS using StartTime as reference (treat entire response as generation time)
		m.TPS, err = CalculateTPS(outputTokens, m.StartTime, m.EndTime)
		if err != nil {
			return err
		}
	}

	// Store in window
	window := c.windows[m.Key]
	if window == nil {
		// This shouldn't happen as StartRequest creates the window
		window = c.getOrCreateWindow(m.Key)
	}

	window.Add(*m)

	// Persist if configured
	if c.persistence != nil {
		metrics := window.GetAll()
		_ = c.persistence.Save(m.Key, metrics) // Ignore persistence errors
	}

	return nil
}

// GetWindowStats returns aggregated statistics for the given metric key.
// It calculates min, max, avg, median, p95, and p99 for TPS, TTFT, and TPOT.
// Returns WindowStats and true if the key exists and has data, otherwise returns zero WindowStats and false.
func (c *TPSCollector) GetWindowStats(key MetricKey) (WindowStats, bool) {
	window, exists := c.windows[key]
	if !exists || window.Len() == 0 {
		return WindowStats{Key: key}, false
	}

	stats := window.GetStats()
	stats.Key = key
	return stats, true
}

// GetAllKeys returns all metric keys that have recorded data.
// The returned slice is empty if no requests have been completed.
func (c *TPSCollector) GetAllKeys() []MetricKey {
	keys := make([]MetricKey, 0, len(c.windows))
	for key := range c.windows {
		window := c.windows[key]
		if window != nil && window.Len() > 0 {
			keys = append(keys, key)
		}
	}
	return keys
}

// getOrCreateWindow gets an existing window or creates a new one for the given key.
// This is an internal method that is not safe for concurrent use from multiple goroutines.
// The caller must hold appropriate locks if needed.
func (c *TPSCollector) getOrCreateWindow(key MetricKey) *SlidingWindow {
	window, exists := c.windows[key]
	if !exists {
		window = NewSlidingWindow()
		c.windows[key] = window

		// Try to load persisted data if persistence is configured
		if c.persistence != nil {
			if loaded, err := c.persistence.Load(key); err == nil && len(loaded) > 0 {
				// Restore window state from persisted data
				window.RestoreFrom(loaded)
			}
		}
	}
	return window
}