package metrics

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateTTFT(t *testing.T) {
	tests := []struct {
		name         string
		startTime    time.Time
		firstToken   time.Time
		expectedTTFT float64
		expectedErr  error
	}{
		{
			name:         "normal streaming request",
			startTime:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			firstToken:   time.Date(2024, 1, 1, 0, 0, 0, 150_000_000, time.UTC), // 0.15s
			expectedTTFT: 0.15,
			expectedErr:  nil,
		},
		{
			name:         "zero duration - same time",
			startTime:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			firstToken:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedTTFT: 0,
			expectedErr:  nil,
		},
		{
			name:         "non-streaming - first token is zero",
			startTime:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			firstToken:   time.Time{},
			expectedTTFT: 0,
			expectedErr:  ErrFirstTokenTimeNotSet,
		},
		{
			name:         "longer duration",
			startTime:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			firstToken:   time.Date(2024, 1, 1, 0, 0, 1, 500_000_000, time.UTC), // 1.5s
			expectedTTFT: 1.5,
			expectedErr:  nil,
		},
		{
			name:         "negative duration - first token before start",
			startTime:    time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC),
			firstToken:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedTTFT: 0,
			expectedErr:  ErrInvalidTokenOrder,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ttft, err := CalculateTTFT(tt.startTime, tt.firstToken)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedErr), "expected error to wrap ErrFirstTokenTimeNotSet")
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tt.expectedTTFT, ttft, 0.001, "TTFT should match expected value")
			}
		})
	}
}

func TestCalculateTPS(t *testing.T) {
	tests := []struct {
		name           string
		outputTokens   int
		firstTokenTime time.Time
		endTime        time.Time
		expectedTPS    float64
		expectedErr    error
	}{
		{
			name:           "normal TPS calculation",
			outputTokens:   100,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC), // 2s
			expectedTPS:    50.00,
			expectedErr:    nil,
		},
		{
			name:           "fractional TPS",
			outputTokens:   33,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC), // 2s
			expectedTPS:    16.50,
			expectedErr:    nil,
		},
		{
			name:           "zero output tokens",
			outputTokens:   0,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC),
			expectedTPS:    0,
			expectedErr:    ErrNoOutputTokens,
		},
		{
			name:           "negative output tokens",
			outputTokens:   -10,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC),
			expectedTPS:    0,
			expectedErr:    ErrNoOutputTokens,
		},
		{
			name:           "zero generation time",
			outputTokens:   100,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedTPS:    0,
			expectedErr:    ErrInvalidGenerationTime,
		},
		{
			name:           "negative generation time",
			outputTokens:   100,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC),
			expectedTPS:    0,
			expectedErr:    ErrInvalidGenerationTime,
		},
		{
			name:           "fast generation",
			outputTokens:   100,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 0, 500_000_000, time.UTC), // 0.5s
			expectedTPS:    200.00,
			expectedErr:    nil,
		},
		{
			name:           "TPS rounding down",
			outputTokens:   1,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 0, 449_000_000, time.UTC), // 0.449s
			expectedTPS:    2.23, // 1/0.449 = 2.227... → 2.23
			expectedErr:    nil,
		},
		{
			name:           "TPS rounding up",
			outputTokens:   1,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 0, 451_000_000, time.UTC), // 0.451s
			expectedTPS:    2.22, // 1/0.451 = 2.217... → 2.22
			expectedErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tps, err := CalculateTPS(tt.outputTokens, tt.firstTokenTime, tt.endTime)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedErr), "expected error to be ErrNoOutputTokens or ErrInvalidGenerationTime")
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tt.expectedTPS, tps, 0.01, "TPS should match expected value with 2 decimal precision")
			}
		})
	}
}

func TestCalculateTPOT(t *testing.T) {
	tests := []struct {
		name           string
		outputTokens   int
		firstTokenTime time.Time
		endTime        time.Time
		expectedTPOT   float64
		expectedErr    error
	}{
		{
			name:           "normal TPOT calculation",
			outputTokens:   100,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC), // 2s
			expectedTPOT:   0.02,
			expectedErr:    nil,
		},
		{
			name:           "single token",
			outputTokens:   1,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 0, 50_000_000, time.UTC), // 0.05s
			expectedTPOT:   0.05,
			expectedErr:    nil,
		},
		{
			name:           "zero output tokens",
			outputTokens:   0,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC),
			expectedTPOT:   0,
			expectedErr:    ErrNoOutputTokens,
		},
		{
			name:           "negative output tokens",
			outputTokens:   -10,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 2, 0, time.UTC),
			expectedTPOT:   0,
			expectedErr:    ErrNoOutputTokens,
		},
		{
			name:           "zero generation time",
			outputTokens:   100,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedTPOT:   0,
			expectedErr:    nil, // TPOT allows zero generation time (all tokens arrived at once)
		},
		{
			name:           "slow generation",
			outputTokens:   10,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 5, 0, time.UTC), // 5s
			expectedTPOT:   0.5,
			expectedErr:    nil,
		},
		{
			name:           "fast generation",
			outputTokens:   1000,
			firstTokenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:        time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC), // 1s
			expectedTPOT:   0.001,
			expectedErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpot, err := CalculateTPOT(tt.outputTokens, tt.firstTokenTime, tt.endTime)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedErr), "expected error to be ErrNoOutputTokens")
			} else {
				assert.NoError(t, err)
				assert.InDelta(t, tt.expectedTPOT, tpot, 0.001, "TPOT should match expected value")
			}
		})
	}
}

func TestValidateMetrics(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		metrics     *RequestMetrics
		expectedErr error
	}{
		{
			name: "valid streaming metrics",
			metrics: &RequestMetrics{
				TrackingID:     "test-001",
				Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true},
				StartTime:      baseTime,
				FirstTokenTime: baseTime.Add(150 * time.Millisecond),
				EndTime:        baseTime.Add(2 * time.Second),
				OutputTokens:   100,
			},
			expectedErr: nil,
		},
		{
			name: "valid non-streaming metrics",
			metrics: &RequestMetrics{
				TrackingID:     "test-002",
				Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: false},
				StartTime:      baseTime,
				FirstTokenTime: time.Time{}, // Zero value for non-streaming
				EndTime:        baseTime.Add(2 * time.Second),
				OutputTokens:   100,
			},
			expectedErr: nil,
		},
		{
			name: "missing start time",
			metrics: &RequestMetrics{
				TrackingID:     "test-003",
				Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true},
				StartTime:      time.Time{},
				FirstTokenTime: baseTime.Add(150 * time.Millisecond),
				EndTime:        baseTime.Add(2 * time.Second),
				OutputTokens:   100,
			},
			expectedErr: ErrStartTimeNotSet,
		},
		{
			name: "missing end time",
			metrics: &RequestMetrics{
				TrackingID:     "test-004",
				Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true},
				StartTime:      baseTime,
				FirstTokenTime: baseTime.Add(150 * time.Millisecond),
				EndTime:        time.Time{},
				OutputTokens:   100,
			},
			expectedErr: ErrEndTimeNotSet,
		},
		{
			name: "end time before start time",
			metrics: &RequestMetrics{
				TrackingID:     "test-005",
				Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true},
				StartTime:      baseTime.Add(2 * time.Second),
				FirstTokenTime: baseTime.Add(150 * time.Millisecond),
				EndTime:        baseTime,
				OutputTokens:   100,
			},
			expectedErr: ErrInvalidTimeRange,
		},
		{
			name: "negative output tokens",
			metrics: &RequestMetrics{
				TrackingID:     "test-006",
				Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true},
				StartTime:      baseTime,
				FirstTokenTime: baseTime.Add(150 * time.Millisecond),
				EndTime:        baseTime.Add(2 * time.Second),
				OutputTokens:   -5,
			},
			expectedErr: ErrInvalidOutputTokens,
		},
		{
			name: "zero output tokens",
			metrics: &RequestMetrics{
				TrackingID:     "test-007",
				Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true},
				StartTime:      baseTime,
				FirstTokenTime: baseTime.Add(150 * time.Millisecond),
				EndTime:        baseTime.Add(2 * time.Second),
				OutputTokens:   0,
			},
			expectedErr: nil, // Zero tokens is allowed for failed/empty requests
		},
		{
			name: "first token time before start time (streaming)",
			metrics: &RequestMetrics{
				TrackingID:     "test-008",
				Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true},
				StartTime:      baseTime.Add(1 * time.Second),
				FirstTokenTime: baseTime.Add(150 * time.Millisecond),
				EndTime:        baseTime.Add(2 * time.Second),
				OutputTokens:   100,
			},
			expectedErr: ErrInvalidTokenOrder, // FirstTokenTime before StartTime is invalid
		},
		{
			name: "first token time after end time",
			metrics: &RequestMetrics{
				TrackingID:     "test-009",
				Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true},
				StartTime:      baseTime,
				FirstTokenTime: baseTime.Add(3 * time.Second),
				EndTime:        baseTime.Add(2 * time.Second),
				OutputTokens:   100,
			},
			expectedErr: ErrInvalidTokenOrder, // FirstTokenTime after EndTime is invalid
		},
		{
			name: "nil metrics",
			metrics:     nil,
			expectedErr: ErrNilMetrics,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMetrics(tt.metrics)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedErr), "expected error to match")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCalculateMetricsIntegration(t *testing.T) {
	// Integration test that verifies all calculations work together
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	firstTokenTime := startTime.Add(150 * time.Millisecond)
	endTime := startTime.Add(2 * time.Second)
	outputTokens := 100

	metrics := &RequestMetrics{
		TrackingID:     "integration-test",
		Key:            MetricKey{Provider: "openai", Model: "gpt-4", Streaming: true},
		StartTime:      startTime,
		FirstTokenTime: firstTokenTime,
		EndTime:        endTime,
		OutputTokens:   outputTokens,
	}

	// Validate first
	assert.NoError(t, ValidateMetrics(metrics))

	// Calculate TTFT
	ttft, err := CalculateTTFT(startTime, firstTokenTime)
	assert.NoError(t, err)
	assert.InDelta(t, 0.15, ttft, 0.001)

	// Calculate TPS: 100 tokens / 1.85s = 54.05 tokens/s
	tps, err := CalculateTPS(outputTokens, firstTokenTime, endTime)
	assert.NoError(t, err)
	assert.InDelta(t, 54.05, tps, 0.01)

	// Calculate TPOT: 1.85s / 100 tokens = 0.0185s/token
	tpot, err := CalculateTPOT(outputTokens, firstTokenTime, endTime)
	assert.NoError(t, err)
	assert.InDelta(t, 0.0185, tpot, 0.001)

	// Verify the relationship: TPS = 1 / TPOT
	// With floating point precision, they should be approximately equal
	product := tps * tpot
	assert.InDelta(t, 1.0, product, 0.01, "TPS * TPOT should equal 1.0")
}

func TestEdgeCases(t *testing.T) {
	t.Run("very large token count", func(t *testing.T) {
		startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		endTime := startTime.Add(10 * time.Second)
		tokens := 100000 // 100k tokens

		tps, err := CalculateTPS(tokens, startTime, endTime)
		assert.NoError(t, err)
		assert.InDelta(t, 10000.00, tps, 0.1)

		tpot, err := CalculateTPOT(tokens, startTime, endTime)
		assert.NoError(t, err)
		assert.InDelta(t, 0.0001, tpot, 0.00001)
	})

	t.Run("nanosecond precision", func(t *testing.T) {
		startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		endTime := startTime.Add(1 * time.Nanosecond)

		// This should not cause division by zero or panic
		tpot, err := CalculateTPOT(1, startTime, endTime)
		assert.NoError(t, err)
		assert.True(t, tpot > 0, "TPOT should be positive for 1 nanosecond generation")
	})

	t.Run("extreme TTFT values", func(t *testing.T) {
		// Very fast TTFT
		startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		firstToken := startTime.Add(1 * time.Millisecond)

		ttft, err := CalculateTTFT(startTime, firstToken)
		assert.NoError(t, err)
		assert.InDelta(t, 0.001, ttft, 0.0001)

		// Very slow TTFT
		startTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		firstToken = startTime.Add(1 * time.Minute)

		ttft, err = CalculateTTFT(startTime, firstToken)
		assert.NoError(t, err)
		assert.InDelta(t, 60.0, ttft, 0.1)
	})
}

func TestRoundingBehavior(t *testing.T) {
	// Test that TPS rounding follows math.Round(x*100)/100 pattern
	testCases := []struct {
		input    float64
		expected float64
	}{
		{16.49999999, 16.50}, // Rounds up at 0.005 boundary
		{16.50000000, 16.50}, // Exactly at boundary
		{16.50000001, 16.50}, // Just above boundary
		{16.50499999, 16.50}, // Just below 0.005
		{16.50500000, 16.51}, // Rounds up at 0.005
		{16.50999999, 16.51}, // Rounds down at 0.00499999
		{16.51500000, 16.52}, // Rounds up
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%f", tc.input), func(t *testing.T) {
			result := math.Round(tc.input*100) / 100
			assert.Equal(t, tc.expected, result)
		})
	}
}