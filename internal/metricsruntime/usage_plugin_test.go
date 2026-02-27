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
		state.SetProvider("anthropic")
		state.SetModel("claude-3.5")
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
		if captured.Provider != record.Provider {
			t.Fatalf("expected persisted Provider=%q, got %q", record.Provider, captured.Provider)
		}
		if captured.Model != record.Model {
			t.Fatalf("expected persisted Model=%q, got %q", record.Model, captured.Model)
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
	state.SetProvider("anthropic")
	state.SetModel("claude-3.5")
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

	snap := state.Snapshot()
	key := metrics.MetricKey{Provider: snap.Provider, Model: snap.Model, Streaming: snap.Streaming}
	ws, ok := p.collector.GetWindowStats(key)
	if ok || ws.Count != 0 {
		t.Fatalf("expected canceled request to not enter TPSCollector window, got ok=%v count=%d", ok, ws.Count)
	}
}

// TestHandleUsage_E2EWithZeroOutputButReasoningTokens verifies that E2E stats
// are recorded when outputTokens=0 but reasoningTokens>0 (i.e. generatedTokens>0).
// RED: current code guards E2E on outputTokens>0 so this must FAIL.
func TestHandleUsage_E2EWithZeroOutputButReasoningTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origEnqueue := enqueueMetricRecord
	defer func() { enqueueMetricRecord = origEnqueue }()
	enqueueMetricRecord = func(r metricspersist.MetricRecord) {}

	p := NewMetricsPlugin(nil)

	rec := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(rec)
	state := NewRequestState(false, "claude-3.5")
	state.SetProvider("anthropic")
	state.SetModel("claude-3.5")
	state.StartedAt = time.Now().Add(-1 * time.Second)
	state.SetStatusCode(200)
	AttachRequestState(ginCtx, state)

	ctx := logging.WithRequestID(context.Background(), "req_e2e_reasoning")
	ctx = context.WithValue(ctx, "gin", ginCtx)
	record := usage.Record{
		Provider: "anthropic",
		Model:    "claude-3.5",
		Detail: usage.Detail{
			InputTokens:     50,
			OutputTokens:    0,
			ReasoningTokens: 20,
			TotalTokens:     70,
		},
	}

	p.HandleUsage(ctx, record)

	snap := state.Snapshot()
	if snap.WindowStatsE2E.Count <= 0 {
		t.Fatalf("expected WindowStatsE2E.Count > 0 (generatedTokens=20), got %d", snap.WindowStatsE2E.Count)
	}
}

// TestHandleUsage_E2ENoOutputNoGenerated verifies that E2E stats are NOT
// recorded when both outputTokens and reasoningTokens are 0 (generatedTokens=0).
func TestHandleUsage_E2ENoOutputNoGenerated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origEnqueue := enqueueMetricRecord
	defer func() { enqueueMetricRecord = origEnqueue }()
	enqueueMetricRecord = func(r metricspersist.MetricRecord) {}

	p := NewMetricsPlugin(nil)

	rec := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(rec)
	state := NewRequestState(false, "claude-3.5")
	state.SetProvider("anthropic")
	state.SetModel("claude-3.5")
	state.StartedAt = time.Now().Add(-1 * time.Second)
	state.SetStatusCode(200)
	AttachRequestState(ginCtx, state)

	ctx := logging.WithRequestID(context.Background(), "req_e2e_nogen")
	ctx = context.WithValue(ctx, "gin", ginCtx)
	record := usage.Record{
		Provider: "anthropic",
		Model:    "claude-3.5",
		Detail: usage.Detail{
			InputTokens:     50,
			OutputTokens:    0,
			ReasoningTokens: 0,
			TotalTokens:     50,
		},
	}

	p.HandleUsage(ctx, record)

	snap := state.Snapshot()
	if snap.WindowStatsE2E.Count != 0 {
		t.Fatalf("expected WindowStatsE2E.Count == 0 (no generated tokens), got %d", snap.WindowStatsE2E.Count)
	}
}

func TestMetricsPlugin_HandleUsage_PopulatesCollectorWindowStatsForNonCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origEnqueue := enqueueMetricRecord
	defer func() { enqueueMetricRecord = origEnqueue }()
	// Avoid persistence side effects; this test only needs the in-memory collector.
	enqueueMetricRecord = func(r metricspersist.MetricRecord) {}

	t.Run("baseline: record provider/model (when state matches or is empty)", func(t *testing.T) {
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
	})

	t.Run("record provider/model overrides state for collector key", func(t *testing.T) {
		p := NewMetricsPlugin(nil)

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		state := NewRequestState(false, "gpt-5.2")
		// Handler pre-sets provider/model (e.g. claude code handler sets "claude").
		// The record from the actual executor should always win.
		state.SetProvider("anthropic")
		state.SetModel("claude-3.5")
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

		snap := state.Snapshot()
		// After HandleUsage the state should reflect the record's actual provider/model.
		if snap.Provider != record.Provider {
			t.Fatalf("expected state.Provider=%q after HandleUsage, got %q", record.Provider, snap.Provider)
		}
		if snap.Model != record.Model {
			t.Fatalf("expected state.Model=%q after HandleUsage, got %q", record.Model, snap.Model)
		}

		// Collector key must be based on the record (actual executor) values.
		keyRecord := metrics.MetricKey{Provider: record.Provider, Model: record.Model, Streaming: snap.Streaming}
		wsRecord, okRecord := p.collector.GetWindowStats(keyRecord)
		if !okRecord {
			t.Fatalf("expected record-key collector GetWindowStats ok=true")
		}
		if wsRecord.Count <= 0 {
			t.Fatalf("expected record-key collector window count > 0, got %d", wsRecord.Count)
		}

		// The old state key (anthropic/claude-3.5) should NOT have been used.
		keyOldState := metrics.MetricKey{Provider: "anthropic", Model: "claude-3.5", Streaming: snap.Streaming}
		wsOld, okOld := p.collector.GetWindowStats(keyOldState)
		if okOld && wsOld.Count > 0 {
			t.Fatalf("expected old-state key to not be used for aggregation, got ok=%v count=%d", okOld, wsOld.Count)
		}
	})
}
