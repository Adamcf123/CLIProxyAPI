package metricsruntime

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricslog"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

// MetricsPlugin implements sdk/cliproxy/usage.Plugin to calculate performance
// metrics (TPS, TTFT, TPOT) and persist them as structured JSONL logs.
type MetricsPlugin struct {
	collector *metrics.TPSCollector
}

// NewMetricsPlugin creates a new MetricsPlugin that writes to the provided collector.
func NewMetricsPlugin(collector *metrics.TPSCollector) *MetricsPlugin {
	if collector == nil {
		collector = metrics.NewCollector(nil)
	}
	return &MetricsPlugin{collector: collector}
}

// HandleUsage implements usage.Plugin.
// It computes metrics from usage record and request state, then enqueues a log line.
func (p *MetricsPlugin) HandleUsage(ctx context.Context, record usage.Record) {
	if p == nil {
		return
	}

	now := time.Now().UTC()

	// Extract tokens from record detail.
	inputTokens := record.Detail.InputTokens
	outputTokens := record.Detail.OutputTokens
	totalTokens := record.Detail.TotalTokens

	// Detect "usage missing": all tokens are 0 and request did not fail.
	usageMissing := inputTokens == 0 && outputTokens == 0 && totalTokens == 0 && !record.Failed

	// Prepare log line with available data.
	line := metricslog.MetricsLogLine{
		TrackingID: record.AuthID, // fallback; may be overwritten below
		Provider:   record.Provider,
		Model:      record.Model,
		Timestamp:  now,
	}

	if !usageMissing {
		if inputTokens != 0 {
			v := inputTokens
			line.InputTokens = &v
		}
		if outputTokens != 0 {
			v := outputTokens
			line.OutputTokens = &v
		}
		if totalTokens != 0 {
			v := totalTokens
			line.TotalTokens = &v
		}
	}

	// Try to get gin context and request state for richer metrics.
	var state *RequestState
	if ginCtx, ok := ctx.Value("gin").(*gin.Context); ok && ginCtx != nil {
		state, _ = GetRequestState(ginCtx)
		if state != nil {
			// Prefer state tracking ID if available.
			if state.TrackingID != "" {
				line.TrackingID = state.TrackingID
			}
			line.RequestPath = &state.RequestPath
			if state.StatusCode != 0 {
				code := int64(state.StatusCode)
				line.StatusCode = &code
			}
			if state.LastError != "" {
				line.ErrorInfo = &state.LastError
			}
		}
	}

	// Compute TPS/TTFT/TPOT when we have output tokens and sufficient timing data.
	if outputTokens > 0 && state != nil {
		key := metrics.MetricKey{
			Provider:  record.Provider,
			Model:     record.Model,
			Streaming: state.Streaming,
		}

		// Build RequestMetrics for calculation.
		m := &metrics.RequestMetrics{
			TrackingID: line.TrackingID,
			Key:        key,
			StartTime:  state.StartedAt,
			EndTime:    now,
		}
		if state.FirstTokenAt != nil {
			m.FirstTokenTime = *state.FirstTokenAt
		}

		// Only proceed if we have valid timing.
		canCompute := false
		if state.Streaming {
			// Streaming requires first token time.
			canCompute = state.FirstTokenAt != nil && !state.FirstTokenAt.IsZero()
		} else {
			// Non-streaming only requires start time.
			canCompute = !state.StartedAt.IsZero()
		}

		if canCompute {
			// CompleteRequest calculates TTFT, TPS, TPOT and stores in sliding window.
			// We intentionally ignore errors here (fail-silent for logging path).
			_ = p.collector.CompleteRequest(m, int(outputTokens))

			// Populate log line with calculated metrics.
			if m.TPS != 0 {
				v := m.TPS
				line.TPS = &v
			}
			if m.TTFT != 0 {
				v := m.TTFT
				line.TTFT = &v
			}
			if m.TPOT != 0 {
				v := m.TPOT
				line.TPOT = &v
			}

			// Backfill state for display/summary purposes.
			state.SetMetrics(m)
		}
	}

	// Calculate duration if we have start time.
	if state != nil && !state.StartedAt.IsZero() {
		duration := now.Sub(state.StartedAt).Milliseconds()
		line.DurationMS = &duration
	}

	// Enqueue for async write (non-blocking, drops on full queue).
	metricslog.Enqueue(line)
}
