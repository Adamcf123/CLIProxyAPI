// Package metrics provides unit tests for the TPSCollector.
// Tests cover request tracking, metric calculation, window aggregation,
// and edge cases like zero tokens and invalid requests.
package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTPSCollector_StartRequest verifies that StartRequest creates a new RequestMetrics
// with the correct key and start time.
func TestTPSCollector_StartRequest(t *testing.T) {
	collector := NewCollector(nil)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: true,
	}

	metrics := collector.StartRequest(key)

	require.NotNil(t, metrics)
	assert.Equal(t, key, metrics.Key)
	assert.False(t, metrics.StartTime.IsZero())
	assert.True(t, metrics.FirstTokenTime.IsZero()) // FirstTokenTime not set yet
	assert.True(t, metrics.EndTime.IsZero())        // EndTime not set yet
	assert.Equal(t, 0, metrics.OutputTokens)
}

// TestTPSCollector_CompleteRequest_Streaming verifies that CompleteRequest correctly
// calculates metrics for streaming requests and stores them in the window.
func TestTPSCollector_CompleteRequest_Streaming(t *testing.T) {
	collector := NewCollector(nil)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: true,
	}

	metrics := collector.StartRequest(key)

	// Simulate some processing time before first token
	time.Sleep(10 * time.Millisecond)
	collector.RecordFirstToken(metrics)

	// Simulate token generation time
	time.Sleep(20 * time.Millisecond)
	outputTokens := 100

	err := collector.CompleteRequest(metrics, outputTokens)
	require.NoError(t, err)

	// Verify metrics were calculated
	assert.Greater(t, metrics.TTFT, 0.0)  // Time to first token
	assert.Greater(t, metrics.TPOT, 0.0)  // Time per output token
	assert.Greater(t, metrics.TPS, 0.0)   // Tokens per second
	assert.Equal(t, outputTokens, metrics.OutputTokens)

	// Verify metrics were added to window
	stats, exists := collector.GetWindowStats(key)
	require.True(t, exists)
	assert.Equal(t, 1, stats.Count)
}

// TestTPSCollector_CompleteRequest_NonStreaming verifies that CompleteRequest correctly
// calculates metrics for non-streaming requests (TTFT = total response time).
func TestTPSCollector_CompleteRequest_NonStreaming(t *testing.T) {
	collector := NewCollector(nil)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: false,
	}

	metrics := collector.StartRequest(key)

	// Simulate total response time (no first token time for non-streaming)
	time.Sleep(30 * time.Millisecond)
	outputTokens := 50

	err := collector.CompleteRequest(metrics, outputTokens)
	require.NoError(t, err)

	// Verify metrics were calculated
	assert.Greater(t, metrics.TTFT, 0.0) // Should equal total response time
	assert.Greater(t, metrics.TPOT, 0.0) // Average time per token
	assert.Greater(t, metrics.TPS, 0.0)  // Tokens per second
	assert.Equal(t, outputTokens, metrics.OutputTokens)

	// FirstTokenTime should remain zero for non-streaming
	assert.True(t, metrics.FirstTokenTime.IsZero())

	// Verify metrics were added to window
	stats, exists := collector.GetWindowStats(key)
	require.True(t, exists)
	assert.Equal(t, 1, stats.Count)
}

// TestTPSCollector_CompleteRequest_ZeroTokens verifies that CompleteRequest returns an error
// when outputTokens is 0, and the request is not recorded in the window.
func TestTPSCollector_CompleteRequest_ZeroTokens(t *testing.T) {
	collector := NewCollector(nil)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: true,
	}

	metrics := collector.StartRequest(key)
	collector.RecordFirstToken(metrics)

	// Try to complete with 0 tokens
	err := collector.CompleteRequest(metrics, 0)
	require.Error(t, err)
	assert.Equal(t, ErrNonPositiveTokens, err)

	// Verify request was NOT added to window
	stats, exists := collector.GetWindowStats(key)
	assert.False(t, exists)
	assert.Equal(t, 0, stats.Count)
}

// TestTPSCollector_CompleteRequest_NilMetrics verifies that CompleteRequest returns an error
// when metrics is nil.
func TestTPSCollector_CompleteRequest_NilMetrics(t *testing.T) {
	collector := NewCollector(nil)

	err := collector.CompleteRequest(nil, 100)
	require.Error(t, err)
	assert.Equal(t, ErrNilMetrics, err)
}

// TestTPSCollector_CompleteRequest_StreamingWithoutFirstToken verifies that CompleteRequest
// returns an error for streaming requests when FirstTokenTime is not set.
func TestTPSCollector_CompleteRequest_StreamingWithoutFirstToken(t *testing.T) {
	collector := NewCollector(nil)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: true,
	}

	metrics := collector.StartRequest(key)
	// Do NOT call RecordFirstToken - this should cause an error for streaming requests

	err := collector.CompleteRequest(metrics, 100)
	require.Error(t, err)
	assert.Equal(t, ErrZeroFirstTokenTime, err)
}

// TestTPSCollector_GetWindowStats verifies that GetWindowStats returns correct
// statistics for a window with multiple requests.
func TestTPSCollector_GetWindowStats(t *testing.T) {
	collector := NewCollector(nil)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: true,
	}

	// Add multiple requests with known characteristics
	for i := 0; i < 10; i++ {
		metrics := collector.StartRequest(key)
		time.Sleep(10 * time.Millisecond) // Vary TTFT
		collector.RecordFirstToken(metrics)
		time.Sleep(5 * time.Millisecond)  // Vary generation time
		_ = collector.CompleteRequest(metrics, 50+i*10)
	}

	stats, exists := collector.GetWindowStats(key)
	require.True(t, exists)
	assert.Equal(t, 10, stats.Count)
	assert.Greater(t, stats.TPSMax, stats.TPSMin)
	assert.Greater(t, stats.TTFTMax, stats.TTFTMin)
	assert.Greater(t, stats.TPOTMax, stats.TPOTMin)
}

// TestTPSCollector_GetWindowStats_NonExistentKey verifies that GetWindowStats
// returns false for keys that don't exist.
func TestTPSCollector_GetWindowStats_NonExistentKey(t *testing.T) {
	collector := NewCollector(nil)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: true,
	}

	stats, exists := collector.GetWindowStats(key)
	assert.False(t, exists)
	assert.Equal(t, 0, stats.Count)
}

// TestTPSCollector_GetAllKeys verifies that GetAllKeys returns all keys that
// have recorded data.
func TestTPSCollector_GetAllKeys(t *testing.T) {
	collector := NewCollector(nil)

	keys := []MetricKey{
		{Provider: "openai", Model: "gpt-4", Streaming: true},
		{Provider: "openai", Model: "gpt-4", Streaming: false},
		{Provider: "claude", Model: "claude-3-opus", Streaming: true},
	}

	// Add requests for each key
	for _, key := range keys {
		metrics := collector.StartRequest(key)
		if key.Streaming {
			collector.RecordFirstToken(metrics)
		}
		time.Sleep(5 * time.Millisecond)
		_ = collector.CompleteRequest(metrics, 10)
	}

	// Create a key that was started but never completed
	uncompletedKey := MetricKey{Provider: "gemini", Model: "gemini-pro", Streaming: true}
	_ = collector.StartRequest(uncompletedKey) // Don't complete this one

	// GetAllKeys should only return keys with completed requests
	allKeys := collector.GetAllKeys()
	assert.Len(t, allKeys, 3)

	// Verify all expected keys are present
	keySet := make(map[MetricKey]bool)
	for _, k := range allKeys {
		keySet[k] = true
	}
	for _, expected := range keys {
		assert.True(t, keySet[expected], "Key %v should be in GetAllKeys", expected)
	}
	assert.False(t, keySet[uncompletedKey], "Uncompleted key should not be in GetAllKeys")
}

// TestTPSCollector_MultipleWindows verifies that the collector correctly
// maintains separate windows for different keys.
func TestTPSCollector_MultipleWindows(t *testing.T) {
	collector := NewCollector(nil)

	key1 := MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true}
	key2 := MetricKey{Provider: "claude", Model: "claude-3-opus", Streaming: true}

	// Add requests to key1
	for i := 0; i < 5; i++ {
		metrics := collector.StartRequest(key1)
		collector.RecordFirstToken(metrics)
		time.Sleep(5 * time.Millisecond)
		_ = collector.CompleteRequest(metrics, 20)
	}

	// Add requests to key2
	for i := 0; i < 3; i++ {
		metrics := collector.StartRequest(key2)
		collector.RecordFirstToken(metrics)
		time.Sleep(5 * time.Millisecond)
		_ = collector.CompleteRequest(metrics, 30)
	}

	// Verify each key has the correct count
	stats1, exists1 := collector.GetWindowStats(key1)
	require.True(t, exists1)
	assert.Equal(t, 5, stats1.Count)

	stats2, exists2 := collector.GetWindowStats(key2)
	require.True(t, exists2)
	assert.Equal(t, 3, stats2.Count)
}

// TestTPSCollector_SlidingWindowCapacity verifies that the sliding window
// correctly limits to 100 requests and overwrites oldest entries.
func TestTPSCollector_SlidingWindowCapacity(t *testing.T) {
	collector := NewCollector(nil)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: true,
	}

	// Add 105 requests (exceeds capacity of 100)
	for i := 0; i < 105; i++ {
		metrics := collector.StartRequest(key)
		collector.RecordFirstToken(metrics)
		time.Sleep(1 * time.Millisecond)
		_ = collector.CompleteRequest(metrics, 10)
	}

	stats, exists := collector.GetWindowStats(key)
	require.True(t, exists)
	assert.Equal(t, 100, stats.Count) // Should be capped at windowSize
}

// TestTPSCollector_ConcurrentAccess verifies that the collector is safe for
// concurrent access from multiple goroutines.
func TestTPSCollector_ConcurrentAccess(t *testing.T) {
	collector := NewCollector(nil)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: true,
	}

	// Launch multiple goroutines that add requests concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				metrics := collector.StartRequest(key)
				collector.RecordFirstToken(metrics)
				time.Sleep(1 * time.Millisecond)
				_ = collector.CompleteRequest(metrics, 10)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all requests were recorded
	stats, exists := collector.GetWindowStats(key)
	require.True(t, exists)
	assert.Equal(t, 100, stats.Count)
}

// TestTPSCollector_WithPersistence verifies that the collector correctly
// uses the persistence layer when configured.
func TestTPSCollector_WithPersistence(t *testing.T) {
	// Create a mock persistence that records saves
	saveCount := 0
	mockPersistence := &mockPersistence{
		saveFunc: func(key MetricKey, window []RequestMetrics) error {
			saveCount++
			return nil
		},
		loadFunc: func(key MetricKey) ([]RequestMetrics, error) {
			return nil, nil // No persisted data
		},
	}

	collector := NewCollector(mockPersistence)

	key := MetricKey{
		Provider:  "openai",
		Model:     "gpt-4",
		Streaming: true,
	}

	// Add 5 requests
	for i := 0; i < 5; i++ {
		metrics := collector.StartRequest(key)
		collector.RecordFirstToken(metrics)
		time.Sleep(1 * time.Millisecond)
		_ = collector.CompleteRequest(metrics, 10)
	}

	// Verify persistence was called for each request
	assert.Equal(t, 5, saveCount)
}

// mockPersistence is a mock implementation of Persistence for testing.
type mockPersistence struct {
	saveFunc func(key MetricKey, window []RequestMetrics) error
	loadFunc func(key MetricKey) ([]RequestMetrics, error)
}

func (m *mockPersistence) Save(key MetricKey, window []RequestMetrics) error {
	if m.saveFunc != nil {
		return m.saveFunc(key, window)
	}
	return nil
}

func (m *mockPersistence) Load(key MetricKey) ([]RequestMetrics, error) {
	if m.loadFunc != nil {
		return m.loadFunc(key)
	}
	return nil, nil
}