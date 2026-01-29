// Package metrics provides TPS (Tokens Per Second) calculation and aggregation
// for LLM API proxy requests. It tracks TPS, TTFT (Time To First Token),
// and TPOT (Time Per Output Token) metrics grouped by provider, model, and streaming mode.
package metrics

import (
	"errors"
	"math"
	"time"
)

var (
	// ErrZeroFirstTokenTime is returned when FirstTokenTime is zero (non-streaming case needs special handling)
	ErrZeroFirstTokenTime = errors.New("first token time is zero")

	// ErrNonPositiveGenerationTime is returned when token generation time is not positive
	ErrNonPositiveGenerationTime = errors.New("token generation time must be positive")

	// ErrNonPositiveTokens is returned when token count is zero or negative
	ErrNonPositiveTokens = errors.New("output tokens must be positive")

	// ErrMissingStartTime is returned when StartTime is not set
	ErrMissingStartTime = errors.New("start time is required")

	// ErrMissingEndTime is returned when EndTime is not set
	ErrMissingEndTime = errors.New("end time is required")

	// ErrInvalidTimeOrder is returned when EndTime is before StartTime
	ErrInvalidTimeOrder = errors.New("end time must be after start time")

	// ErrInvalidTokenOrder is returned when FirstTokenTime is before StartTime
	ErrInvalidTokenOrder = errors.New("first token time must be after start time")

	// Aliases for backward compatibility with tests
	ErrFirstTokenTimeNotSet  = ErrZeroFirstTokenTime
	ErrInvalidGenerationTime = ErrNonPositiveGenerationTime
	ErrNoOutputTokens        = ErrNonPositiveTokens
	ErrStartTimeNotSet       = ErrMissingStartTime
	ErrEndTimeNotSet         = ErrMissingEndTime
	ErrInvalidTimeRange      = ErrInvalidTimeOrder
	ErrInvalidOutputTokens   = ErrNonPositiveTokens
	ErrNilMetrics            = errors.New("metrics is nil")
)

// CalculateTTFT calculates Time To First Token.
// Formula: TTFT = FirstTokenTime - StartTime (in seconds)
// Returns error if FirstTokenTime is zero value.
func CalculateTTFT(startTime, firstTokenTime time.Time) (float64, error) {
	if firstTokenTime.IsZero() {
		return 0, ErrZeroFirstTokenTime
	}

	if startTime.IsZero() {
		return 0, ErrMissingStartTime
	}

	if firstTokenTime.Before(startTime) {
		return 0, ErrInvalidTokenOrder
	}

	duration := firstTokenTime.Sub(startTime)
	return duration.Seconds(), nil
}

// CalculateTPS calculates Tokens Per Second.
// Formula: TPS = OutputTokens / (EndTime - FirstTokenTime)
// The result is rounded to 2 decimal places.
// Returns error if:
// - outputTokens is not positive
// - generation time (EndTime - FirstTokenTime) is not positive
func CalculateTPS(outputTokens int, firstTokenTime, endTime time.Time) (float64, error) {
	if outputTokens <= 0 {
		return 0, ErrNonPositiveTokens
	}

	if firstTokenTime.IsZero() {
		return 0, ErrZeroFirstTokenTime
	}

	if endTime.IsZero() {
		return 0, ErrMissingEndTime
	}

	generationTime := endTime.Sub(firstTokenTime)
	if generationTime <= 0 {
		return 0, ErrNonPositiveGenerationTime
	}

	tps := float64(outputTokens) / generationTime.Seconds()
	// Round to 2 decimal places
	return math.Round(tps*100) / 100, nil
}

// CalculateTPOT calculates Time Per Output Token.
// Formula: TPOT = (EndTime - FirstTokenTime) / OutputTokens (in seconds per token)
// Returns error if outputTokens is not positive.
func CalculateTPOT(outputTokens int, firstTokenTime, endTime time.Time) (float64, error) {
	if outputTokens <= 0 {
		return 0, ErrNonPositiveTokens
	}

	if firstTokenTime.IsZero() {
		return 0, ErrZeroFirstTokenTime
	}

	if endTime.IsZero() {
		return 0, ErrMissingEndTime
	}

	generationTime := endTime.Sub(firstTokenTime)
	return generationTime.Seconds() / float64(outputTokens), nil
}

// ValidateMetrics validates that a RequestMetrics instance has all required fields
// and that the timing relationships are valid.
// Returns nil if valid, otherwise returns a descriptive error.
func ValidateMetrics(m *RequestMetrics) error {
	if m == nil {
		return ErrNilMetrics
	}

	if m.StartTime.IsZero() {
		return ErrMissingStartTime
	}

	if m.EndTime.IsZero() {
		return ErrMissingEndTime
	}

	if m.EndTime.Before(m.StartTime) {
		return ErrInvalidTimeOrder
	}

	// For streaming requests, FirstTokenTime must be set and between StartTime and EndTime
	if m.Key.Streaming {
		if m.FirstTokenTime.IsZero() {
			return ErrZeroFirstTokenTime
		}
		if m.FirstTokenTime.Before(m.StartTime) {
			return ErrInvalidTokenOrder
		}
		if m.FirstTokenTime.After(m.EndTime) {
			return ErrInvalidTokenOrder
		}
	}

	// OutputTokens must be non-negative (can be 0 for failed requests)
	if m.OutputTokens < 0 {
		return ErrInvalidOutputTokens
	}

	return nil
}
