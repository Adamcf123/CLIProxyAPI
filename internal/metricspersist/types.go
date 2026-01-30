package metricspersist

// MetricRecord represents one request-level metrics row persisted to SQLite.
//
// Pointer fields are used for values that may be unavailable; they will be
// stored as NULL in the database.
//
// NOTE: request_path is intentionally excluded (sensitive boundary).
type MetricRecord struct {
	RequestID string
	Provider  string
	Model     string

	TPS  *float64
	TTFT *float64
	TPOT *float64

	InputTokens  *int64
	OutputTokens *int64
	TotalTokens  *int64

	DurationMS *int64
	StatusCode *int64
	ErrorInfo  *string
}
