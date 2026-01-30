package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/api/handlers/management"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricspersist"
)

type metricsResponse struct {
	Meta struct {
		Mode          string  `json:"mode"`
		RequestedFrom *string `json:"requested_from"`
		RequestedTo   *string `json:"requested_to"`
		EffectiveFrom string  `json:"effective_from"`
		EffectiveTo   string  `json:"effective_to"`
		Filters       struct {
			Provider  *string `json:"provider"`
			Model     *string `json:"model"`
			Streaming *bool   `json:"streaming"`
		} `json:"filters"`
	} `json:"meta"`
	Data *struct {
		RequestID  string `json:"request_id"`
		Streaming  bool   `json:"streaming"`
		TTFTMillis *int64 `json:"ttft_ms"`
		TPOTMillis *int64 `json:"tpot_ms"`
		CreatedAt  string `json:"created_at"`
	} `json:"data"`
	Error string `json:"error"`
}

func setupMetricsRouter(t *testing.T, h *management.Handler) *gin.Engine {
	t.Helper()
	r := gin.New()
	mgmt := r.Group("/v0/management")
	mgmt.GET("/metrics", h.GetMetrics)
	return r
}

func seedMetricsDB(t *testing.T, dbPath string) {
	t.Helper()

	db, err := metricspersist.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := metricspersist.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const insert = `INSERT INTO metrics (
		request_id,
		provider,
		model,
		streaming,
		tps,
		ttft,
		tpot,
		input_tokens,
		output_tokens,
		total_tokens,
		duration_ms,
		status_code,
		error_info,
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	_, err = db.Exec(
		insert,
		"req_1",
		"openai",
		"gpt-4o",
		1,
		12.5,
		1.234,
		0.005,
		10,
		20,
		30,
		1234,
		200,
		nil,
		"2026-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert metrics row: %v", err)
	}
}

func TestManagementMetrics_RequestIDFound(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil)

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)
	h.SetNowUTC(func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })

	r := setupMetricsRouter(t, h)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=req_1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp metricsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Meta.Mode != "request_id" {
		t.Fatalf("meta.mode: got %q want %q", resp.Meta.Mode, "request_id")
	}
	if resp.Data == nil {
		t.Fatalf("expected data")
	}
	if resp.Data.RequestID != "req_1" {
		t.Fatalf("data.request_id: got %q want %q", resp.Data.RequestID, "req_1")
	}
	if resp.Data.Streaming != true {
		t.Fatalf("data.streaming: got %t want %t", resp.Data.Streaming, true)
	}
	if resp.Data.TTFTMillis == nil || *resp.Data.TTFTMillis != 1234 {
		t.Fatalf("data.ttft_ms: got=%v want=%d", resp.Data.TTFTMillis, 1234)
	}
	if resp.Data.TPOTMillis == nil || *resp.Data.TPOTMillis != 5 {
		t.Fatalf("data.tpot_ms: got=%v want=%d", resp.Data.TPOTMillis, 5)
	}
}

func TestManagementMetrics_RequestIDNotFound(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil)

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)

	r := setupMetricsRouter(t, h)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, w.Code, w.Body.String())
	}

	var resp metricsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == "" {
		t.Fatalf("expected error message")
	}
}

func TestManagementMetrics_StreaminBoolValidation(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil)

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)

	r := setupMetricsRouter(t, h)

	bad := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=req_1&streaming=maybe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, bad)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}

	good := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=req_1&streaming=true", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, good)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	good = httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=req_1&streaming=1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, good)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestManagementMetrics_ModeValidation(t *testing.T) {
	h := management.NewHandler(&config.Config{}, "", nil)
	r := setupMetricsRouter(t, h)

	bad := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?mode=wat", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, bad)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestManagementMetrics_TimeRangeValidation(t *testing.T) {
	h := management.NewHandler(&config.Config{}, "", nil)
	r := setupMetricsRouter(t, h)

	fromOnly := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?mode=percentiles&from=2026-01-01T00:00:00Z", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, fromOnly)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}

	invalidRFC := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?mode=percentiles&from=not-a-time&to=2026-01-01T00:00:00Z", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, invalidRFC)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestManagementMetrics_DefaultTimeRangeMetaEcho(t *testing.T) {
	h := management.NewHandler(&config.Config{}, "", nil)
	h.SetNowUTC(func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })
	r := setupMetricsRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?mode=percentiles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented && w.Code != http.StatusOK {
		t.Fatalf("expected status %d or %d, got %d: %s", http.StatusNotImplemented, http.StatusOK, w.Code, w.Body.String())
	}

	var resp metricsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Meta.RequestedFrom != nil || resp.Meta.RequestedTo != nil {
		t.Fatalf("expected requested_from/to to be null")
	}
	if resp.Meta.EffectiveTo != "2026-01-01T00:00:00Z" {
		t.Fatalf("meta.effective_to: got %q want %q", resp.Meta.EffectiveTo, "2026-01-01T00:00:00Z")
	}
	if resp.Meta.EffectiveFrom != "2025-12-31T23:00:00Z" {
		t.Fatalf("meta.effective_from: got %q want %q", resp.Meta.EffectiveFrom, "2025-12-31T23:00:00Z")
	}
}
