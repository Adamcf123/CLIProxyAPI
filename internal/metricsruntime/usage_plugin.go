package metricsruntime

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricspersist"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func metricKeyFromStateOrRecord(snap RequestStateSnapshot, record usage.Record) (metrics.MetricKey, bool) {
	provider := strings.TrimSpace(snap.Provider)
	if provider == "" {
		provider = strings.TrimSpace(record.Provider)
	}
	model := strings.TrimSpace(snap.Model)
	if model == "" {
		model = strings.TrimSpace(record.Model)
	}
	if provider == "" || model == "" {
		return metrics.MetricKey{}, false
	}
	return metrics.MetricKey{Provider: provider, Model: model, Streaming: snap.Streaming}, true
}

var enqueueMetricRecord = metricspersist.Enqueue

const (
	minOutputTokensForRates        = 16
	minStreamingContentTokenChunks = 2
	minNonStreamingTotalDuration   = 300 * time.Millisecond
	// Keep a small floor to avoid extreme TPS spikes from near-zero durations.
	// With event-aware content chunk counting, we can be less strict than 300ms and
	// still suppress the "single flush at the end" buffered-stream pattern.
	minStreamingPostFirstTokenDuration = 50 * time.Millisecond
)

// MetricsPlugin implements sdk/cliproxy/usage.Plugin to calculate performance
// metrics (TPS, TTFT, TPOT) and persist them to SQLite.
type MetricsPlugin struct {
	collector *metrics.TPSCollector
	e2e       *metrics.E2ECollector

	errorsMu    sync.Mutex
	errorsTotal map[metrics.MetricKey]int
}

// NewMetricsPlugin creates a new MetricsPlugin that writes to the provided collector.
func NewMetricsPlugin(collector *metrics.TPSCollector) *MetricsPlugin {
	if collector == nil {
		collector = metrics.NewCollector(nil)
	}
	return &MetricsPlugin{
		collector:   collector,
		e2e:         metrics.NewE2ECollector(),
		errorsTotal: make(map[metrics.MetricKey]int),
	}
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
	generatedTokens := int(outputTokens) + int(record.Detail.ReasoningTokens)

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

	var durationMs int64

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
	var key metrics.MetricKey
	hasKey := false
	if ginCtx, ok := ctx.Value("gin").(*gin.Context); ok && ginCtx != nil {
		state, _ = GetRequestState(ginCtx)
		if state != nil {
			snap = state.Snapshot()
			// display.go reads provider/model from RequestStateSnapshot. Always prefer the
			// usage record's values because they reflect the actual executor that handled the
			// request, which may differ from the handler's initial guess (e.g. a request
			// arriving at /v1/messages with a non-Claude model that gets routed to codex).
			if strings.TrimSpace(record.Provider) != "" {
				state.SetProvider(record.Provider)
				snap.Provider = record.Provider
			}
			if strings.TrimSpace(record.Model) != "" {
				state.SetModel(record.Model)
				snap.Model = record.Model
			}
			isCanceled = snap.IsClientCanceled()
			key, hasKey = metricKeyFromStateOrRecord(snap, record)
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
	if outputTokens > 0 && state != nil && !isCanceled && hasKey {
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

	// Add sliding window stats for the current provider/model/streaming key.
	if state != nil && hasKey {
		ws, ok := p.collector.GetWindowStats(key)
		state.SetWindowStats(windowStatsFromCollector(ws, ok))
		state.SetErrorsTotal(p.updateErrorsTotal(key, snap))
	}

	// Calculate duration if we have start time.
	if state != nil && !state.StartedAt.IsZero() {
		durationMs = now.Sub(state.StartedAt).Milliseconds()
		durationMSPtr = &durationMs
	}

	// Compute end-to-end rates (based on output_tokens / total duration). This is a separate
	// signal from streaming token-timing TPS/TPOT and is expected to be available more often.
	if state != nil && hasKey && !isCanceled && generatedTokens > 0 && durationMs > 0 {
		statusCode := int64(0)
		if statusCodePtr != nil {
			statusCode = *statusCodePtr
		}
		isFailure := record.Failed || snap.LastError != "" || statusCode >= 400
		if !isFailure {
			secs := float64(durationMs) / 1000.0
			if secs > 0 {
				v := float64(generatedTokens) / secs
				tpse2e := &v
				w := secs / float64(generatedTokens)
				tpote2e := &w
				state.SetE2EMetrics(tpse2e, tpote2e)

				p.e2e.Add(key, *tpse2e, *tpote2e)
				count, tpsAvg, tpotAvg, ok := p.e2e.GetAverages(key)
				if ok {
					v1 := tpsAvg
					v2 := tpotAvg
					state.SetWindowStatsE2E(RequestWindowStatsE2E{Count: count, TPSE2EAvg: &v1, TPOTE2EAvg: &v2})
				}
			}
		}
	}

	// Persist to SQLite asynchronously (off request path).
	// Duplicates are handled by the DB unique constraint on request_id.
	if requestID := logging.GetRequestID(ctx); requestID != "" {
		enqueueMetricRecord(metricspersist.MetricRecord{
			RequestID:    requestID,
			Provider:     provider,
			Model:        model,
			Streaming:    &snap.Streaming,
			TPSGen:       tps,
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

func windowStatsFromCollector(stats metrics.WindowStats, ok bool) RequestWindowStats {
	if !ok || stats.Count <= 0 {
		return RequestWindowStats{Count: 0, TPSSampleCount: 0, TTFTSampleCount: 0, TPOTSampleCount: 0}
	}
	// WindowStats uses concrete float64 fields; metrics_summary uses pointers so an empty
	// window can express avg=null instead of 0.
	tps := stats.TPSAvg
	ttft := stats.TTFTAvg
	tpot := stats.TPOTAvg
	return RequestWindowStats{
		Count:           stats.Count,
		TPSAvg:          &tps,
		TTFTAvg:         &ttft,
		TPOTAvg:         &tpot,
		TPSSampleCount:  stats.Count,
		TTFTSampleCount: stats.Count,
		TPOTSampleCount: stats.Count,
	}
}

func (p *MetricsPlugin) updateErrorsTotal(key metrics.MetricKey, snap RequestStateSnapshot) int {
	if p == nil {
		return 0
	}
	p.errorsMu.Lock()
	defer p.errorsMu.Unlock()
	if p.errorsTotal == nil {
		p.errorsTotal = make(map[metrics.MetricKey]int)
	}

	total := p.errorsTotal[key]
	if snap.IsFailure() && !snap.IsClientCanceled() {
		total++
		p.errorsTotal[key] = total
	}
	return total
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
