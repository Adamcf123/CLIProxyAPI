package metricsruntime

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricspersist"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestMetricsPlugin_StreamingWithContentTokens_ComputesRates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestID := "0000000000000def"

	dbPath := filepath.Join(t.TempDir(), "metrics.sqlite")
	db, err := metricspersist.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := metricspersist.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	origEnqueue := enqueueMetricRecord
	defer func() { enqueueMetricRecord = origEnqueue }()

	var insertErr error
	enqueueMetricRecord = func(r metricspersist.MetricRecord) {
		if insertErr != nil {
			return
		}
		insertErr = insertMetricRecord(db, r)
	}

	rec := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(rec)

	state := NewRequestState(true, "gpt-5.2")
	// Force a stable timing window: start 2s ago, first token 1s ago.
	state.StartedAt = time.Now().Add(-2 * time.Second)
	state.SetStatusCode(200)
	AttachRequestState(ginCtx, state)

	firstTokenAt := time.Now().Add(-1 * time.Second)
	MaybeRecordFirstContentToken(ginCtx, []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"), firstTokenAt)
	MaybeRecordFirstContentToken(ginCtx, []byte("data: {\"choices\":[{\"delta\":{\"content\":\"!\"}}]}\n\n"), firstTokenAt.Add(10*time.Millisecond))

	ctx := logging.WithRequestID(context.Background(), requestID)
	ctx = context.WithValue(ctx, "gin", ginCtx)

	p := NewMetricsPlugin(nil)
	p.HandleUsage(ctx, usage.Record{
		Provider: "openai",
		Model:    "gpt-5.2",
		Failed:   false,
		Detail: usage.Detail{
			InputTokens:  10,
			OutputTokens: 100,
			TotalTokens:  110,
		},
	})

	if insertErr != nil {
		t.Fatalf("insertMetricRecord: %v", insertErr)
	}

	var (
		tps  sql.NullFloat64
		ttft sql.NullFloat64
		tpot sql.NullFloat64
	)
	err = db.QueryRow(
		`SELECT tps, ttft, tpot FROM metrics WHERE request_id = ?;`,
		requestID,
	).Scan(&tps, &ttft, &tpot)
	if err == sql.ErrNoRows {
		t.Fatalf("expected a persisted metrics row for request_id=%s", requestID)
	}
	if err != nil {
		t.Fatalf("query persisted row: %v", err)
	}

	if !ttft.Valid {
		t.Fatalf("expected ttft to be non-NULL for streaming with first token")
	}
	if !tps.Valid {
		t.Fatalf("expected tps to be non-NULL for streaming with sufficient evidence")
	}
	if !tpot.Valid {
		t.Fatalf("expected tpot to be non-NULL for streaming with sufficient evidence")
	}
}
