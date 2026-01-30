package metricsruntime

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
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
