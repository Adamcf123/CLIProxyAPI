package metricsruntime

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricspersist"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestMetricsPlugin_HandleUsage_MapsLastErrorToErrorInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origEnqueue := enqueueMetricRecord
	defer func() { enqueueMetricRecord = origEnqueue }()

	var captured *metricspersist.MetricRecord
	enqueueMetricRecord = func(r metricspersist.MetricRecord) {
		rc := r
		captured = &rc
	}

	p := NewMetricsPlugin(nil)
	record := usage.Record{Provider: "openai", Model: "gpt-5.2"}

	t.Run("with last error", func(t *testing.T) {
		captured = nil
		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)

		state := NewRequestState(true, "gpt-5.2")
		state.SetLastError(errors.New("boom"))
		AttachRequestState(ginCtx, state)

		ctx := logging.WithRequestID(context.Background(), "req_test")
		ctx = context.WithValue(ctx, "gin", ginCtx)
		p.HandleUsage(ctx, record)

		if captured == nil {
			t.Fatalf("expected MetricRecord to be enqueued")
		}
		if captured.RequestID != "req_test" {
			t.Fatalf("expected RequestID=\"req_test\", got %q", captured.RequestID)
		}
		if captured.Streaming == nil || *captured.Streaming != true {
			t.Fatalf("expected Streaming=true")
		}
		if captured.ErrorInfo == nil || *captured.ErrorInfo != "boom" {
			got := "<nil>"
			if captured.ErrorInfo != nil {
				got = *captured.ErrorInfo
			}
			t.Fatalf("expected ErrorInfo=\"boom\", got %q", got)
		}
	})

	t.Run("without last error", func(t *testing.T) {
		captured = nil
		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)

		state := NewRequestState(true, "gpt-5.2")
		AttachRequestState(ginCtx, state)

		ctx := logging.WithRequestID(context.Background(), "req_test")
		ctx = context.WithValue(ctx, "gin", ginCtx)
		p.HandleUsage(ctx, record)

		if captured == nil {
			t.Fatalf("expected MetricRecord to be enqueued")
		}
		if captured.ErrorInfo != nil {
			t.Fatalf("expected ErrorInfo to be nil when LastError is empty")
		}
	})
}

func TestMetricsPlugin_HandleUsage_CanceledPersists499AndNilErrorInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origEnqueue := enqueueMetricRecord
	defer func() { enqueueMetricRecord = origEnqueue }()

	var captured *metricspersist.MetricRecord
	enqueueMetricRecord = func(r metricspersist.MetricRecord) {
		rc := r
		captured = &rc
	}

	p := NewMetricsPlugin(nil)

	rec := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(rec)
	state := NewRequestState(false, "gpt-5.2")
	// Ensure the non-canceled path would be able to compute rates if it ran.
	state.StartedAt = time.Now().Add(-1 * time.Second)
	state.SetStatusCode(200)
	state.MarkClientCanceled()
	AttachRequestState(ginCtx, state)

	ctx := logging.WithRequestID(context.Background(), "req_test")
	ctx = context.WithValue(ctx, "gin", ginCtx)
	record := usage.Record{
		Provider: "openai",
		Model:    "gpt-5.2",
		Detail: usage.Detail{
			OutputTokens: 100,
			TotalTokens:  100,
		},
	}
	// Sanity: canceled is expressed only via status_code=499, not LastError.
	if snap := state.Snapshot(); !snap.IsClientCanceled() || snap.LastError != "" {
		t.Fatalf("expected precondition: canceled=true and LastError empty")
	}

	p.HandleUsage(ctx, record)

	if captured == nil {
		t.Fatalf("expected MetricRecord to be enqueued")
	}
	if captured.StatusCode == nil || *captured.StatusCode != 499 {
		got := int64(0)
		if captured.StatusCode != nil {
			got = *captured.StatusCode
		}
		t.Fatalf("expected StatusCode=499, got %d", got)
	}
	if captured.ErrorInfo != nil {
		t.Fatalf("expected ErrorInfo to be nil for canceled")
	}

	// canceled requests must not be aggregated into TPSCollector-derived metrics.
	if snap := state.Snapshot(); snap.Metrics != nil {
		t.Fatalf("expected RequestState.Metrics to remain nil for canceled request")
	}

	key := metrics.MetricKey{Provider: record.Provider, Model: record.Model, Streaming: state.Snapshot().Streaming}
	ws, ok := p.collector.GetWindowStats(key)
	if ok || ws.Count != 0 {
		t.Fatalf("expected canceled request to not enter TPSCollector window, got ok=%v count=%d", ok, ws.Count)
	}
}

func TestMetricsPlugin_HandleUsage_PopulatesCollectorWindowStatsForNonCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origEnqueue := enqueueMetricRecord
	defer func() { enqueueMetricRecord = origEnqueue }()
	// Avoid persistence side effects; this test only needs the in-memory collector.
	enqueueMetricRecord = func(r metricspersist.MetricRecord) {}

	p := NewMetricsPlugin(nil)

	rec := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(rec)
	state := NewRequestState(false, "gpt-5.2")
	state.StartedAt = time.Now().Add(-1 * time.Second)
	state.SetStatusCode(200)
	AttachRequestState(ginCtx, state)

	ctx := context.WithValue(context.Background(), "gin", ginCtx)
	record := usage.Record{
		Provider: "openai",
		Model:    "gpt-5.2",
		Detail: usage.Detail{
			OutputTokens: 100,
			TotalTokens:  100,
		},
	}

	p.HandleUsage(ctx, record)

	key := metrics.MetricKey{Provider: record.Provider, Model: record.Model, Streaming: state.Snapshot().Streaming}
	ws, ok := p.collector.GetWindowStats(key)
	if !ok {
		t.Fatalf("expected collector GetWindowStats ok=true")
	}
	if ws.Count <= 0 {
		t.Fatalf("expected collector window count > 0, got %d", ws.Count)
	}
}
