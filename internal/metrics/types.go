// Package metrics provides TPS (Tokens Per Second) calculation and aggregation
// for LLM API proxy requests. It tracks TPS, TTFT (Time To First Token),
// and TPOT (Time Per Output Token) metrics grouped by provider, model, and streaming mode.
package metrics

import "time"

// MetricKey represents the grouping key for metrics aggregation.
// Metrics are collected separately for each unique combination of provider, model, and streaming mode.
type MetricKey struct {
	// Provider is the LLM provider name (e.g., "openai", "claude", "gemini")
	Provider string `json:"provider"`

	// Model is the model identifier (e.g., "gpt-4", "claude-3-opus")
	Model string `json:"model"`

	// Streaming indicates whether this is a streaming request.
	// Metrics are aggregated separately for streaming and non-streaming requests.
	Streaming bool `json:"streaming"`
}

// RequestMetrics captures metrics for a single API request.
// It tracks timing information and calculated performance metrics.
type RequestMetrics struct {
	// TrackingID uniquely identifies this request for tracing purposes
	TrackingID string `json:"tracking_id"`

	// Key is the grouping key for this request's metrics
	Key MetricKey `json:"key"`

	// StartTime is when the request was initiated
	StartTime time.Time `json:"start_time"`

	// FirstTokenTime is when the first output token was received.
	// Used to calculate TTFT (Time To First Token).
	FirstTokenTime time.Time `json:"first_token_time"`

	// EndTime is when the request completed (all tokens received)
	EndTime time.Time `json:"end_time"`

	// OutputTokens is the number of tokens generated in the response.
	// This comes from the provider's usage.completion_tokens field.
	OutputTokens int `json:"output_tokens"`

	// TTFT (Time To First Token) is the duration from request start to first token,
	// expressed in seconds. Calculated as FirstTokenTime - StartTime.
	TTFT float64 `json:"ttft"`

	// TPOT (Time Per Output Token) is the average time to generate each token,
	// expressed in seconds per token. Calculated as (EndTime - FirstTokenTime) / OutputTokens.
	TPOT float64 `json:"tpot"`

	// TPS (Tokens Per Second) is the throughput metric, calculated as output tokens
	// divided by token generation time. Expressed with 2 decimal places.
	// Formula: OutputTokens / (EndTime - FirstTokenTime)
	TPS float64 `json:"tps"`
}

// WindowStats represents aggregated statistics over a sliding window of requests.
// It provides percentile-based summaries of TPS, TTFT, and TPOT metrics.
type WindowStats struct {
	// Key is the grouping key for these statistics
	Key MetricKey `json:"key"`

	// Count is the number of requests in this window
	Count int `json:"count"`

	// TPS statistics
	TPSMin  float64 `json:"tps_min"`
	TPSMax  float64 `json:"tps_max"`
	TPSAvg  float64 `json:"tps_avg"`
	TPSMedian float64 `json:"tps_median"`
	TPSP95  float64 `json:"tps_p95"`
	TPSP99  float64 `json:"tps_p99"`

	// TTFT statistics (in seconds)
	TTFTMin  float64 `json:"ttft_min"`
	TTFTMax  float64 `json:"ttft_max"`
	TTFTAvg  float64 `json:"ttft_avg"`
	TTFTMedian float64 `json:"ttft_median"`
	TTFTP95  float64 `json:"ttft_p95"`
	TTFTP99  float64 `json:"ttft_p99"`

	// TPOT statistics (in seconds per token)
	TPOTMin  float64 `json:"tpot_min"`
	TPOTMax  float64 `json:"tpot_max"`
	TPOTAvg  float64 `json:"tpot_avg"`
	TPOTMedian float64 `json:"tpot_median"`
	TPOTP95  float64 `json:"tpot_p95"`
	TPOTP99  float64 `json:"tpot_p99"`
}
