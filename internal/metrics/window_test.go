// Package metrics provides tests for sliding window aggregation
package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSlidingWindow_AddAndGet verifies basic Add and GetAll functionality.
func TestSlidingWindow_AddAndGet(t *testing.T) {
	window := NewSlidingWindow()

	// Add 5 metrics
	for i := 1; i <= 5; i++ {
		m := RequestMetrics{
			TPS:  float64(i * 10),
			TTFT: float64(i) * 0.1,
			TPOT: float64(i) * 0.05,
			Key: MetricKey{
				Provider:  "test-provider",
				Model:     "test-model",
				Streaming: true,
			},
			StartTime:      time.Now(),
			FirstTokenTime: time.Now(),
			EndTime:        time.Now(),
			OutputTokens:   i * 10,
		}
		window.Add(m)
	}

	// Verify length
	assert.Equal(t, 5, window.Len(), "Len() should return 5")

	// Verify GetAll returns 5 elements
	metrics := window.GetAll()
	require.Len(t, metrics, 5, "GetAll() should return 5 elements")

	// Verify values in order
	assert.Equal(t, 10.0, metrics[0].TPS)
	assert.Equal(t, 20.0, metrics[1].TPS)
	assert.Equal(t, 30.0, metrics[2].TPS)
	assert.Equal(t, 40.0, metrics[3].TPS)
	assert.Equal(t, 50.0, metrics[4].TPS)
}

// TestSlidingWindow_CircularBuffer verifies circular buffer behavior when exceeding capacity.
func TestSlidingWindow_CircularBuffer(t *testing.T) {
	window := NewSlidingWindow()

	// Add 105 metrics (exceeds capacity of 100)
	for i := 0; i < 105; i++ {
		m := RequestMetrics{
			TPS: float64(i),
			Key: MetricKey{
				Provider:  "test",
				Model:     "model",
				Streaming: false,
			},
		}
		window.Add(m)
	}

	// Verify length is capped at 100
	assert.Equal(t, 100, window.Len(), "Len() should be capped at 100")

	// Verify the first 5 elements (indices 0-4) were overwritten
	metrics := window.GetAll()
	require.Len(t, metrics, 100)

	// The oldest element should now be index 5 (first element overwritten)
	assert.Equal(t, 5.0, metrics[0].TPS, "First 5 elements should be overwritten")
	// The newest element should be index 104
	assert.Equal(t, 104.0, metrics[99].TPS, "Last element should be the last added")
}

// TestSlidingWindow_GetStats verifies statistical calculations.
func TestSlidingWindow_GetStats(t *testing.T) {
	window := NewSlidingWindow()

	// Add metrics with known values
	tpsValues := []float64{10, 20, 30, 40, 50}
	for _, tps := range tpsValues {
		m := RequestMetrics{
			TPS:  tps,
			TTFT: tps * 0.1,  // 1, 2, 3, 4, 5
			TPOT: tps * 0.05, // 0.5, 1, 1.5, 2, 2.5
			Key: MetricKey{
				Provider:  "test",
				Model:     "model",
				Streaming: true,
			},
			StartTime:      time.Now(),
			FirstTokenTime: time.Now(),
			EndTime:        time.Now(),
			OutputTokens:   100,
		}
		window.Add(m)
	}

	stats := window.GetStats()

	// Verify TPS statistics
	assert.Equal(t, 5, stats.Count, "Count should be 5")
	assert.Equal(t, 10.0, stats.TPSMin, "TPS Min should be 10")
	assert.Equal(t, 50.0, stats.TPSMax, "TPS Max should be 50")
	assert.InDelta(t, 30.0, stats.TPSAvg, 0.01, "TPS Avg should be 30")
	assert.Equal(t, 30.0, stats.TPSMedian, "TPS Median should be 30")
	// For 5 values, p95 index = 3.8 (between 40 and 50)
	// Interpolated value = 40*(1-0.8) + 50*0.8 = 48
	assert.InDelta(t, 48.0, stats.TPSP95, 0.01, "TPS P95 should be 48")
	// For 5 values, p99 index = 3.96 (between 40 and 50)
	// Interpolated value = 40*(1-0.96) + 50*0.96 = 49.6
	assert.InDelta(t, 49.6, stats.TPSP99, 0.01, "TPS P99 should be 49.6")

	// Verify TTFT statistics
	assert.Equal(t, 1.0, stats.TTFTMin, "TTFT Min should be 1")
	assert.Equal(t, 5.0, stats.TTFTMax, "TTFT Max should be 5")
	assert.InDelta(t, 3.0, stats.TTFTAvg, 0.01, "TTFT Avg should be 3")
	assert.Equal(t, 3.0, stats.TTFTMedian, "TTFT Median should be 3")

	// Verify TPOT statistics
	assert.InDelta(t, 0.5, stats.TPOTMin, 0.01, "TPOT Min should be 0.5")
	assert.InDelta(t, 2.5, stats.TPOTMax, 0.01, "TPOT Max should be 2.5")
	assert.InDelta(t, 1.5, stats.TPOTAvg, 0.01, "TPOT Avg should be 1.5")
	assert.InDelta(t, 1.5, stats.TPOTMedian, 0.01, "TPOT Median should be 1.5")
}

// TestSlidingWindow_Empty verifies empty window returns zero stats.
func TestSlidingWindow_Empty(t *testing.T) {
	window := NewSlidingWindow()

	// Empty window should have length 0
	assert.Equal(t, 0, window.Len(), "Empty window Len() should be 0")

	// GetAll should return nil or empty slice
	metrics := window.GetAll()
	assert.Nil(t, metrics, "GetAll() on empty window should return nil")

	// GetStats should return zero WindowStats
	stats := window.GetStats()
	assert.Equal(t, 0, stats.Count, "Stats Count should be 0")
	assert.Equal(t, 0.0, stats.TPSMin, "Stats TPSMin should be 0")
	assert.Equal(t, 0.0, stats.TPSMax, "Stats TPSMax should be 0")
	assert.Equal(t, 0.0, stats.TPSAvg, "Stats TPSAvg should be 0")
}

// TestSlidingWindow_SingleElement verifies single element statistics.
func TestSlidingWindow_SingleElement(t *testing.T) {
	window := NewSlidingWindow()

	m := RequestMetrics{
		TPS:  100.5,
		TTFT: 1.5,
		TPOT: 0.25,
		Key: MetricKey{
			Provider:  "test",
			Model:     "model",
			Streaming: true,
		},
	}
	window.Add(m)

	stats := window.GetStats()

	// All statistics should equal the single value
	assert.Equal(t, 1, stats.Count)
	assert.Equal(t, 100.5, stats.TPSMin)
	assert.Equal(t, 100.5, stats.TPSMax)
	assert.Equal(t, 100.5, stats.TPSAvg)
	assert.Equal(t, 100.5, stats.TPSMedian)
	assert.Equal(t, 100.5, stats.TPSP95)
	assert.Equal(t, 100.5, stats.TPSP99)

	assert.Equal(t, 1.5, stats.TTFTMin)
	assert.Equal(t, 1.5, stats.TTFTMax)
	assert.Equal(t, 1.5, stats.TTFTAvg)
	assert.Equal(t, 1.5, stats.TTFTMedian)

	assert.Equal(t, 0.25, stats.TPOTMin)
	assert.Equal(t, 0.25, stats.TPOTMax)
	assert.Equal(t, 0.25, stats.TPOTAvg)
	assert.Equal(t, 0.25, stats.TPOTMedian)
}

// TestSlidingWindow_Concurrent verifies thread safety under concurrent access.
func TestSlidingWindow_Concurrent(t *testing.T) {
	window := NewSlidingWindow()
	const numGoroutines = 10
	const numOpsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Multiple goroutines adding metrics concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				m := RequestMetrics{
					TPS: float64(id*numOpsPerGoroutine + j),
					Key: MetricKey{
						Provider:  "test",
						Model:     "model",
						Streaming: true,
					},
				}
				window.Add(m)

				// Also read concurrently
				_ = window.Len()
				_ = window.GetAll()
				_ = window.GetStats()
			}
		}(i)
	}

	wg.Wait()

	// Verify final state
	assert.Equal(t, 100, window.Len(), "Window should be at max capacity")

	// Verify stats can be calculated without deadlock
	stats := window.GetStats()
	assert.Equal(t, 100, stats.Count)
	assert.Greater(t, stats.TPSMin, 0.0)
	assert.Less(t, stats.TPSMax, float64(numGoroutines*numOpsPerGoroutine))
}

// TestSlidingWindow_RestoreFromMetrics verifies persistence restoration.
func TestSlidingWindow_RestoreFromMetrics(t *testing.T) {
	window := NewSlidingWindow()

	// Create persisted data
	persisted := make([]RequestMetrics, 50)
	for i := 0; i < 50; i++ {
		persisted[i] = RequestMetrics{
			TPS: float64(i),
			Key: MetricKey{
				Provider:  "test",
				Model:     "model",
				Streaming: false,
			},
		}
	}

	// Restore from persisted data
	window.RestoreFromMetrics(persisted)

	// Verify restoration
	assert.Equal(t, 50, window.Len())

	metrics := window.GetAll()
	require.Len(t, metrics, 50)

	// Verify values are in correct order
	for i := 0; i < 50; i++ {
		assert.Equal(t, float64(i), metrics[i].TPS)
	}
}

// TestCalculatePercentile verifies percentile calculation accuracy.
func TestCalculatePercentile(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		p        float64
		expected float64
	}{
		{
			name:     "median of even count",
			values:   []float64{1, 2, 3, 4, 5, 6},
			p:        50,
			expected: 3.5, // (3 + 4) / 2
		},
		{
			name:     "median of odd count",
			values:   []float64{1, 2, 3, 4, 5},
			p:        50,
			expected: 3.0, // middle element
		},
		{
			name:     "p95 of sorted values",
			values:   []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        95,
			expected: 9.55, // interpolated
		},
		{
			name:     "p99 of sorted values",
			values:   []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        99,
			expected: 9.91, // interpolated
		},
		{
			name:     "p0 (minimum)",
			values:   []float64{1, 2, 3, 4, 5},
			p:        0,
			expected: 1.0,
		},
		{
			name:     "p100 (maximum)",
			values:   []float64{1, 2, 3, 4, 5},
			p:        100,
			expected: 5.0,
		},
		{
			name:     "single element",
			values:   []float64{42},
			p:        50,
			expected: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentile(tt.values, tt.p)
			assert.InDelta(t, tt.expected, result, 0.01, "Percentile calculation mismatch")
		})
	}
}

// TestCalculateStats verifies overall stats calculation.
func TestCalculateStats(t *testing.T) {
	metrics := []RequestMetrics{
		{TPS: 10, TTFT: 1.0, TPOT: 0.1},
		{TPS: 20, TTFT: 2.0, TPOT: 0.2},
		{TPS: 30, TTFT: 3.0, TPOT: 0.3},
	}

	key := MetricKey{Provider: "test", Model: "model", Streaming: true}
	stats := calculateStats(key, metrics)

	assert.Equal(t, 3, stats.Count)
	assert.Equal(t, key, stats.Key)

	// TPS stats
	assert.Equal(t, 10.0, stats.TPSMin)
	assert.Equal(t, 30.0, stats.TPSMax)
	assert.InDelta(t, 20.0, stats.TPSAvg, 0.01)
	assert.Equal(t, 20.0, stats.TPSMedian)

	// TTFT stats
	assert.Equal(t, 1.0, stats.TTFTMin)
	assert.Equal(t, 3.0, stats.TTFTMax)
	assert.InDelta(t, 2.0, stats.TTFTAvg, 0.01)
	assert.Equal(t, 2.0, stats.TTFTMedian)

	// TPOT stats
	assert.Equal(t, 0.1, stats.TPOTMin)
	assert.Equal(t, 0.3, stats.TPOTMax)
	assert.InDelta(t, 0.2, stats.TPOTAvg, 0.01)
	assert.Equal(t, 0.2, stats.TPOTMedian)
}

// TestCalculateStats_Empty verifies empty metrics handling.
func TestCalculateStats_Empty(t *testing.T) {
	key := MetricKey{Provider: "test", Model: "model", Streaming: true}
	stats := calculateStats(key, []RequestMetrics{})

	assert.Equal(t, 0, stats.Count)
	assert.Equal(t, key, stats.Key)
	assert.Equal(t, 0.0, stats.TPSMin)
	assert.Equal(t, 0.0, stats.TPSMax)
}
