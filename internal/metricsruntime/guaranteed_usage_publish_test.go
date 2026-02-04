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

func TestMetricsPlugin_NoUsageRecord_PersistsQueryableRow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestID := "0000000000000abc"

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
	state.SetStatusCode(200)
	AttachRequestState(ginCtx, state)

	ctx := logging.WithRequestID(context.Background(), requestID)
	ctx = context.WithValue(ctx, "gin", ginCtx)

	p := NewMetricsPlugin(nil)
	p.HandleUsage(ctx, usage.Record{
		Provider: "openai",
		Model:    "gpt-5.2",
		Failed:   false,
		Detail:   usage.Detail{},
	})

	if insertErr != nil {
		t.Fatalf("insertMetricRecord: %v", insertErr)
	}

	var (
		inputTokens  sql.NullInt64
		outputTokens sql.NullInt64
		totalTokens  sql.NullInt64
		tps          sql.NullFloat64
		tpot         sql.NullFloat64
		statusCode   sql.NullInt64
		streaming    sql.NullInt64
		errorInfo    sql.NullString
	)

	err = db.QueryRow(
		`SELECT input_tokens, output_tokens, total_tokens, tps_gen, tpot, status_code, streaming, error_info FROM metrics WHERE request_id = ?;`,
		requestID,
	).Scan(&inputTokens, &outputTokens, &totalTokens, &tps, &tpot, &statusCode, &streaming, &errorInfo)
	if err == sql.ErrNoRows {
		t.Fatalf("expected a persisted metrics row for request_id=%s", requestID)
	}
	if err != nil {
		t.Fatalf("query persisted row: %v", err)
	}

	if inputTokens.Valid || outputTokens.Valid || totalTokens.Valid {
		t.Fatalf("expected token fields to be NULL when usage is missing")
	}
	if tps.Valid || tpot.Valid {
		t.Fatalf("expected token-derived metrics to be NULL when usage is missing")
	}
	if !statusCode.Valid || statusCode.Int64 != 200 {
		t.Fatalf("expected status_code=200, got valid=%v value=%d", statusCode.Valid, statusCode.Int64)
	}
	if !streaming.Valid || streaming.Int64 != 1 {
		t.Fatalf("expected streaming=1, got valid=%v value=%d", streaming.Valid, streaming.Int64)
	}
	if errorInfo.Valid {
		t.Fatalf("expected error_info to be NULL for successful no-usage record")
	}
}

func insertMetricRecord(db *sql.DB, r metricspersist.MetricRecord) error {
	if db == nil {
		return sql.ErrConnDone
	}
	streaming := int64(0)
	if r.Streaming != nil && *r.Streaming {
		streaming = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := db.ExecContext(
		ctx,
		`INSERT INTO metrics (
			request_id,
			provider,
			model,
			streaming,
			tps_gen,
			ttft,
			tpot,
			input_tokens,
			output_tokens,
			total_tokens,
			duration_ms,
			status_code,
			error_info
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(request_id) DO NOTHING;`,
		r.RequestID,
		r.Provider,
		r.Model,
		streaming,
		r.TPSGen,
		r.TTFT,
		r.TPOT,
		r.InputTokens,
		r.OutputTokens,
		r.TotalTokens,
		r.DurationMS,
		r.StatusCode,
		r.ErrorInfo,
	)
	return err
}
