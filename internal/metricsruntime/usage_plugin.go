package metricsruntime

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricspersist"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

var enqueueMetricRecord = metricspersist.Enqueue

const (
	minOutputTokensForRates            = 16
	minStreamingContentTokenChunks     = 2
	minNonStreamingTotalDuration       = 300 * time.Millisecond
	minStreamingPostFirstTokenDuration = 300 * time.Millisecond
)

// MetricsPlugin implements sdk/cliproxy/usage.Plugin to calculate performance
// metrics (TPS, TTFT, TPOT) and persist them to SQLite.
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
	trackingID := record.AuthID // fallback; may be overwritten below
	provider := record.Provider
	model := record.Model

	var (
		tps  *float64
		ttft *float64
		tpot *float64

		inputTokensPtr  *int64
		outputTokensPtr *int64
		totalTokensPtr  *int64

		durationMSPtr *int64
		statusCodePtr *int64
		errorInfoPtr  *string
	)

	if !usageMissing {
		if inputTokens != 0 {
			v := inputTokens
			inputTokensPtr = &v
		}
		if outputTokens != 0 {
			v := outputTokens
			outputTokensPtr = &v
		}
		if totalTokens != 0 {
			v := totalTokens
			totalTokensPtr = &v
		}
	}

	// Try to get gin context and request state for richer metrics.
	var state *RequestState
	var snap RequestStateSnapshot
	isCanceled := false
	if ginCtx, ok := ctx.Value("gin").(*gin.Context); ok && ginCtx != nil {
		state, _ = GetRequestState(ginCtx)
		if state != nil {
			snap = state.Snapshot()
			isCanceled = snap.IsClientCanceled()
			// Prefer state tracking ID if available.
			if snap.TrackingID != "" {
				trackingID = snap.TrackingID
			}
			if isCanceled {
				code := int64(statusClientClosedRequest)
				statusCodePtr = &code
				// canceled must not be expressed via error_info.
				errorInfoPtr = nil
			} else {
				if snap.StatusCode != 0 {
					code := int64(snap.StatusCode)
					statusCodePtr = &code
				} else {
					// In the common path, handlers set RequestState.StatusCode near the tail of
					// the request. Usage publish can run earlier (e.g. at stream end), so we
					// also fall back to the Gin response writer's status when available.
					sc := ginCtx.Writer.Status()
					if sc != 0 {
						code := int64(sc)
						statusCodePtr = &code
					}
				}
				if snap.LastError != "" {
					v := snap.LastError
					errorInfoPtr = &v
				}
			}
			// Backfill state tokens when usage is available so progress/summary can show them.
			if !usageMissing {
				if inputTokens > 0 {
					state.SetInputTokens(int(inputTokens))
				}
				if outputTokens > 0 {
					state.SetOutputTokens(int(outputTokens))
				}
			}
		}
	}
	// TTFT is computable from timestamps even when token usage is missing.
	if state != nil && !snap.StartedAt.IsZero() {
		if snap.Streaming {
			if snap.FirstContentTokenAt != nil {
				secs := snap.FirstContentTokenAt.Sub(snap.StartedAt).Seconds()
				if secs > 0 {
					v := secs
					ttft = &v
				}
			}
		} else {
			secs := now.Sub(snap.StartedAt).Seconds()
			if secs > 0 {
				v := secs
				ttft = &v
			}
		}
	}

	// Compute TPS/TPOT when we have output tokens and sufficient timing data.
	// Note: even when TPS/TPOT are suppressed for confidence reasons, TTFT is still logged above.
	if outputTokens > 0 && state != nil && !isCanceled {
		key := metrics.MetricKey{
			Provider:  record.Provider,
			Model:     record.Model,
			Streaming: snap.Streaming,
		}

		// Build RequestMetrics for calculation.
		m := &metrics.RequestMetrics{
			TrackingID: trackingID,
			Key:        key,
			StartTime:  snap.StartedAt,
			EndTime:    now,
		}
		if snap.FirstContentTokenAt != nil {
			m.FirstTokenTime = *snap.FirstContentTokenAt
		}

		// Only proceed if we have valid timing.
		canCompute := false
		if snap.Streaming {
			// Streaming requires first token time.
			canCompute = snap.FirstContentTokenAt != nil && !snap.FirstContentTokenAt.IsZero()
		} else {
			// Non-streaming only requires start time.
			canCompute = !snap.StartedAt.IsZero()
		}

		if canCompute && shouldComputeRates(snap, outputTokens, now) {
			// CompleteRequest calculates TTFT, TPS, TPOT and stores in sliding window.
			// We intentionally ignore errors here (fail-silent for logging path).
			_ = p.collector.CompleteRequest(m, int(outputTokens))

			// Populate log line with calculated metrics.
			if m.TPS != 0 {
				v := m.TPS
				tps = &v
			}
			if m.TTFT != 0 {
				v := m.TTFT
				ttft = &v
			}
			if m.TPOT != 0 {
				v := m.TPOT
				tpot = &v
			}

			// Backfill state for display/summary purposes.
			state.SetMetrics(m)
		}
	}

	// Calculate duration if we have start time.
	if state != nil && !state.StartedAt.IsZero() {
		duration := now.Sub(state.StartedAt).Milliseconds()
		durationMSPtr = &duration
	}

	// Persist to SQLite asynchronously (off request path).
	// Duplicates are handled by the DB unique constraint on request_id.
	if requestID := logging.GetRequestID(ctx); requestID != "" {
		enqueueMetricRecord(metricspersist.MetricRecord{
			RequestID:    requestID,
			Provider:     provider,
			Model:        model,
			Streaming:    &snap.Streaming,
			TPS:          tps,
			TTFT:         ttft,
			TPOT:         tpot,
			InputTokens:  inputTokensPtr,
			OutputTokens: outputTokensPtr,
			TotalTokens:  totalTokensPtr,
			DurationMS:   durationMSPtr,
			StatusCode:   statusCodePtr,
			ErrorInfo:    errorInfoPtr,
		})
	}
}

func shouldComputeRates(snap RequestStateSnapshot, outputTokens int64, end time.Time) bool {
	if outputTokens < minOutputTokensForRates {
		return false
	}
	if snap.StartedAt.IsZero() {
		return false
	}
	if end.IsZero() {
		return false
	}

	if snap.Streaming {
		if snap.FirstContentTokenAt == nil || snap.FirstContentTokenAt.IsZero() {
			return false
		}
		// If the provider batches almost all output into a single content chunk (or flushes
		// only once near the end), TPS/TPOT become misleading. Require at least 2 content
		// chunks and some post-first-token duration.
		if snap.ContentTokenChunks < minStreamingContentTokenChunks {
			return false
		}
		post := end.Sub(*snap.FirstContentTokenAt)
		if post < minStreamingPostFirstTokenDuration {
			return false
		}
		return true
	}

	total := end.Sub(snap.StartedAt)
	if total < minNonStreamingTotalDuration {
		return false
	}
	return true
}
