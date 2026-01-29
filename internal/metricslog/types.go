package metricslog

import "time"

// MetricsLogLine represents one JSONL log entry written to logs/metrics-YYYY-MM-DD.jsonl.
// Pointer fields are used for values that may be unavailable; they will be encoded as null.
//
// NOTE: JSON tags intentionally use snake_case to match the plan/context field names.
type MetricsLogLine struct {
	TrackingID string `json:"tracking_id"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`

	TPS  *float64 `json:"tps"`
	TTFT *float64 `json:"ttft"`
	TPOT *float64 `json:"tpot"`

	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
	TotalTokens  *int64 `json:"total_tokens"`

	DurationMS *int64 `json:"duration_ms"`

	RequestPath *string `json:"request_path"`
	StatusCode  *int64  `json:"status_code"`
	ErrorInfo   *string `json:"error_info"`

	Timestamp time.Time `json:"timestamp"`
}
