package test

import (
	"encoding/json"
	"math"
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

type percentileFloat struct {
	SampleCount int      `json:"sample_count"`
	P50         *float64 `json:"p50"`
	P95         *float64 `json:"p95"`
	P99         *float64 `json:"p99"`
}

type percentileMillis struct {
	SampleCount int    `json:"sample_count"`
	P50         *int64 `json:"p50"`
	P95         *int64 `json:"p95"`
	P99         *int64 `json:"p99"`
}

type percentilesResponse struct {
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
	Success []struct {
		Provider  string `json:"provider"`
		Model     string `json:"model"`
		Streaming bool   `json:"streaming"`
		Count     int    `json:"count"`
		Metrics   struct {
			TPS        percentileFloat  `json:"tps"`
			TTFTMillis percentileMillis `json:"ttft_ms"`
			TPOTMillis percentileMillis `json:"tpot_ms"`
			DurationMS percentileMillis `json:"duration_ms"`
		} `json:"metrics"`
	} `json:"success"`
	Failure []struct {
		Provider  string `json:"provider"`
		Model     string `json:"model"`
		Streaming bool   `json:"streaming"`
		Count     int    `json:"count"`
		Metrics   struct {
			TPS        percentileFloat  `json:"tps"`
			TTFTMillis percentileMillis `json:"ttft_ms"`
			TPOTMillis percentileMillis `json:"tpot_ms"`
			DurationMS percentileMillis `json:"duration_ms"`
		} `json:"metrics"`
	} `json:"failure"`
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

func seedPercentilesMetricsDB(t *testing.T, dbPath string) {
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

	rows := []struct {
		id        string
		status    any
		errInfo   any
		tps       any
		ttft      any
		tpot      any
		duration  any
		createdAt string
	}{
		// success rows (count=3)
		{id: "s1", status: 200, errInfo: nil, tps: 10.0, ttft: 0.0016, tpot: 0.01, duration: 1000, createdAt: "2026-01-01T00:10:00Z"},
		{id: "s2", status: 200, errInfo: nil, tps: nil, ttft: nil, tpot: 0.02, duration: 2000, createdAt: "2026-01-01T00:20:00Z"},
		{id: "s3", status: 204, errInfo: nil, tps: 30.0, ttft: nil, tpot: nil, duration: 3000, createdAt: "2026-01-01T00:30:00Z"},
		// failure rows (count=2)
		{id: "f1", status: 500, errInfo: nil, tps: nil, ttft: 0.5, tpot: nil, duration: 4000, createdAt: "2026-01-01T00:40:00Z"},
		{id: "f2", status: 200, errInfo: "boom", tps: nil, ttft: nil, tpot: nil, duration: 5000, createdAt: "2026-01-01T00:50:00Z"},
	}

	for _, r := range rows {
		if _, err := db.Exec(
			insert,
			r.id,
			"openai",
			"gpt-4o",
			1,
			r.tps,
			r.ttft,
			r.tpot,
			nil,
			nil,
			nil,
			r.duration,
			r.status,
			r.errInfo,
			r.createdAt,
		); err != nil {
			t.Fatalf("insert metrics row %s: %v", r.id, err)
		}
	}
}

func assertFloatApprox(t *testing.T, got, want float64, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("float mismatch: got=%v want=%v tol=%v", got, want, tol)
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

func TestManagementMetrics_PercentilesMode(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil)

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedPercentilesMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)

	r := setupMetricsRouter(t, h)
	url := "/v0/management/metrics?mode=percentiles&from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z&provider=openai&model=gpt-4o&streaming=true"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp percentilesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Meta.Mode != "percentiles" {
		t.Fatalf("meta.mode: got %q want %q", resp.Meta.Mode, "percentiles")
	}
	if resp.Meta.Filters.Streaming == nil || *resp.Meta.Filters.Streaming != true {
		t.Fatalf("meta.filters.streaming: got=%v want=true", resp.Meta.Filters.Streaming)
	}

	if len(resp.Success) != 1 {
		t.Fatalf("success groups: got %d want %d", len(resp.Success), 1)
	}
	if len(resp.Failure) != 1 {
		t.Fatalf("failure groups: got %d want %d", len(resp.Failure), 1)
	}

	s := resp.Success[0]
	if s.Count != 3 {
		t.Fatalf("success.count: got %d want %d", s.Count, 3)
	}
	if s.Metrics.TPS.SampleCount != 2 {
		t.Fatalf("success.tps.sample_count: got %d want %d", s.Metrics.TPS.SampleCount, 2)
	}
	if s.Metrics.TPS.P50 == nil || s.Metrics.TPS.P95 == nil || s.Metrics.TPS.P99 == nil {
		t.Fatalf("success.tps percentiles expected non-null")
	}
	assertFloatApprox(t, *s.Metrics.TPS.P50, 20.0, 1e-9)
	assertFloatApprox(t, *s.Metrics.TPS.P95, 29.0, 1e-9)
	assertFloatApprox(t, *s.Metrics.TPS.P99, 29.8, 1e-9)

	if s.Metrics.TTFTMillis.SampleCount != 1 {
		t.Fatalf("success.ttft_ms.sample_count: got %d want %d", s.Metrics.TTFTMillis.SampleCount, 1)
	}
	if s.Metrics.TTFTMillis.P50 == nil || *s.Metrics.TTFTMillis.P50 != 2 {
		t.Fatalf("success.ttft_ms.p50: got=%v want=%d", s.Metrics.TTFTMillis.P50, 2)
	}

	if s.Metrics.TPOTMillis.SampleCount != 2 {
		t.Fatalf("success.tpot_ms.sample_count: got %d want %d", s.Metrics.TPOTMillis.SampleCount, 2)
	}
	if s.Metrics.TPOTMillis.P50 == nil || *s.Metrics.TPOTMillis.P50 != 15 {
		t.Fatalf("success.tpot_ms.p50: got=%v want=%d", s.Metrics.TPOTMillis.P50, 15)
	}
	if s.Metrics.TPOTMillis.P95 == nil || *s.Metrics.TPOTMillis.P95 != 20 {
		t.Fatalf("success.tpot_ms.p95: got=%v want=%d", s.Metrics.TPOTMillis.P95, 20)
	}
	if s.Metrics.TPOTMillis.P99 == nil || *s.Metrics.TPOTMillis.P99 != 20 {
		t.Fatalf("success.tpot_ms.p99: got=%v want=%d", s.Metrics.TPOTMillis.P99, 20)
	}

	if s.Metrics.DurationMS.SampleCount != 3 {
		t.Fatalf("success.duration_ms.sample_count: got %d want %d", s.Metrics.DurationMS.SampleCount, 3)
	}
	if s.Metrics.DurationMS.P50 == nil || *s.Metrics.DurationMS.P50 != 2000 {
		t.Fatalf("success.duration_ms.p50: got=%v want=%d", s.Metrics.DurationMS.P50, 2000)
	}
	if s.Metrics.DurationMS.P95 == nil || *s.Metrics.DurationMS.P95 != 2900 {
		t.Fatalf("success.duration_ms.p95: got=%v want=%d", s.Metrics.DurationMS.P95, 2900)
	}
	if s.Metrics.DurationMS.P99 == nil || *s.Metrics.DurationMS.P99 != 2980 {
		t.Fatalf("success.duration_ms.p99: got=%v want=%d", s.Metrics.DurationMS.P99, 2980)
	}

	f := resp.Failure[0]
	if f.Count != 2 {
		t.Fatalf("failure.count: got %d want %d", f.Count, 2)
	}
	if f.Metrics.TPS.SampleCount != 0 {
		t.Fatalf("failure.tps.sample_count: got %d want %d", f.Metrics.TPS.SampleCount, 0)
	}
	if f.Metrics.TPS.P50 != nil || f.Metrics.TPS.P95 != nil || f.Metrics.TPS.P99 != nil {
		t.Fatalf("failure.tps percentiles expected null when no samples")
	}
}
