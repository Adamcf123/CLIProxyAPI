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
		Outcome    string `json:"outcome"`
		Status     string `json:"status"`
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
		CanceledCount *int    `json:"canceled_count"`
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
			TPSGen     percentileFloat  `json:"tps_gen"`
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
			TPSGen     percentileFloat  `json:"tps_gen"`
			TTFTMillis percentileMillis `json:"ttft_ms"`
			TPOTMillis percentileMillis `json:"tpot_ms"`
			DurationMS percentileMillis `json:"duration_ms"`
		} `json:"metrics"`
	} `json:"failure"`
	Error string `json:"error"`
}

type bucketsResponse struct {
	Meta struct {
		Mode          string  `json:"mode"`
		Bucket        string  `json:"bucket"`
		Timezone      string  `json:"timezone"`
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
		Buckets   []struct {
			Start         string `json:"start"`
			Count         int    `json:"count"`
			CanceledCount int    `json:"canceled_count"`
			Metrics       struct {
				TPSGenAvg             *float64 `json:"tps_gen_avg"`
				TPSGenSampleCount     int      `json:"tps_gen_sample_count"`
				TPSE2EAvg             *float64 `json:"tps_e2e_avg"`
				TPSE2ESampleCount     int      `json:"tps_e2e_sample_count"`
				TTFTMillisAvg         *int64   `json:"ttft_ms_avg"`
				TTFTMillisSampleCount int      `json:"ttft_ms_sample_count"`
				TPOTMillisAvg         *int64   `json:"tpot_ms_avg"`
				TPOTMillisSampleCount int      `json:"tpot_ms_sample_count"`
				DurationMSAvg         *int64   `json:"duration_ms_avg"`
				DurationMSSampleCount int      `json:"duration_ms_sample_count"`
			} `json:"metrics"`
		} `json:"buckets"`
	} `json:"success"`
	Failure []struct {
		Provider  string `json:"provider"`
		Model     string `json:"model"`
		Streaming bool   `json:"streaming"`
		Buckets   []struct {
			Start         string `json:"start"`
			Count         int    `json:"count"`
			CanceledCount int    `json:"canceled_count"`
			Metrics       struct {
				TPSGenAvg             *float64 `json:"tps_gen_avg"`
				TPSGenSampleCount     int      `json:"tps_gen_sample_count"`
				TPSE2EAvg             *float64 `json:"tps_e2e_avg"`
				TPSE2ESampleCount     int      `json:"tps_e2e_sample_count"`
				TTFTMillisAvg         *int64   `json:"ttft_ms_avg"`
				TTFTMillisSampleCount int      `json:"ttft_ms_sample_count"`
				TPOTMillisAvg         *int64   `json:"tpot_ms_avg"`
				TPOTMillisSampleCount int      `json:"tpot_ms_sample_count"`
				DurationMSAvg         *int64   `json:"duration_ms_avg"`
				DurationMSSampleCount int      `json:"duration_ms_sample_count"`
			} `json:"metrics"`
		} `json:"buckets"`
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
		tps_gen,
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
		"0000000000000001",
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

	// canceled row: status_code=499 must be classified as outcome=canceled and MUST NOT set envelope.error.
	_, err = db.Exec(
		insert,
		"00000000000000c1",
		"openai",
		"gpt-4o",
		1,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		9999,
		499,
		nil,
		"2026-01-01T00:01:00Z",
	)
	if err != nil {
		t.Fatalf("insert canceled metrics row: %v", err)
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
		tps_gen,
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
		{id: "1111111111111111", status: 200, errInfo: nil, tps: 10.0, ttft: 0.0016, tpot: 0.01, duration: 1000, createdAt: "2026-01-01T00:10:00Z"},
		{id: "2222222222222222", status: 200, errInfo: nil, tps: nil, ttft: nil, tpot: 0.02, duration: 2000, createdAt: "2026-01-01T00:20:00Z"},
		{id: "3333333333333333", status: 204, errInfo: nil, tps: 30.0, ttft: nil, tpot: nil, duration: 3000, createdAt: "2026-01-01T00:30:00Z"},
		// canceled rows are excluded from percentiles but must be counted in meta.canceled_count.
		// Use extreme values so inclusion would visibly change percentiles.
		{id: "cccccccccccccccc", status: 499, errInfo: nil, tps: 1000.0, ttft: 9.9, tpot: 9.9, duration: 999999, createdAt: "2026-01-01T00:35:00Z"},
		// failure rows (count=2)
		{id: "4444444444444444", status: 500, errInfo: nil, tps: nil, ttft: 0.5, tpot: nil, duration: 4000, createdAt: "2026-01-01T00:40:00Z"},
		{id: "5555555555555555", status: 200, errInfo: "boom", tps: nil, ttft: nil, tpot: nil, duration: 5000, createdAt: "2026-01-01T00:50:00Z"},
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

func seedBucketsMetricsDB(t *testing.T, dbPath string) {
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
		tps_gen,
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
		id           string
		status       any
		errInfo      any
		tps          any
		ttft         any
		tpot         any
		duration     any
		outputTokens any
		createdAt    string
	}{
		// Only the middle bucket (00:10-00:15) has data.
		{id: "6666666666666666", status: 200, errInfo: nil, tps: 10.0, ttft: 0.0016, tpot: 0.01, duration: 1234, outputTokens: 20, createdAt: "2026-01-01T00:10:30Z"},
		{id: "7777777777777777", status: 500, errInfo: nil, tps: nil, ttft: 0.5, tpot: nil, duration: 2345, outputTokens: nil, createdAt: "2026-01-01T00:10:45Z"},
		// Streaming failures can still have a 200 status_code, but are classified as failure when error_info is non-empty.
		{id: "8888888888888888", status: 200, errInfo: "boom", tps: nil, ttft: nil, tpot: nil, duration: nil, outputTokens: nil, createdAt: "2026-01-01T00:10:50Z"},
		// canceled rows should not pollute bucket metrics, but must be counted in canceled_count.
		{id: "9999999999999999", status: 499, errInfo: nil, tps: 999.0, ttft: 9.9, tpot: 9.9, duration: 999999, outputTokens: 99999, createdAt: "2026-01-01T00:10:55Z"},
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
			r.outputTokens,
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

func decodeJSONMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func mustMap(t *testing.T, v any, name string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: missing or wrong type (%T)", name, v)
	}
	return m
}

func assertPersistenceMetaMinimalAndSafe(t *testing.T, meta map[string]any) map[string]any {
	t.Helper()
	persistAny, ok := meta["persistence"]
	if !ok {
		t.Fatalf("expected meta.persistence present when degraded")
	}
	persist := mustMap(t, persistAny, "meta.persistence")

	allowed := map[string]struct{}{
		"degraded":         {},
		"dropped_total":    {},
		"last_drop_at":     {},
		"last_drop_reason": {},
	}
	for k := range persist {
		if _, ok := allowed[k]; !ok {
			t.Fatalf("persistence meta leaked extra fields: key=%q", k)
		}
	}
	return persist
}

func TestManagementMetrics_RequestIDFound(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil)

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)
	h.SetNowUTC(func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })

	r := setupMetricsRouter(t, h)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=0000000000000001", nil)
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
	if resp.Data.RequestID != "0000000000000001" {
		t.Fatalf("data.request_id: got %q want %q", resp.Data.RequestID, "0000000000000001")
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
	if resp.Data.Outcome != "success" {
		t.Fatalf("data.outcome: got %q want %q", resp.Data.Outcome, "success")
	}
	if resp.Data.Status != "success" {
		t.Fatalf("data.status: got %q want %q", resp.Data.Status, "success")
	}
}

func TestManagementMetrics_RequestIDCanceledHasNoEnvelopeError(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil)

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)
	h.SetNowUTC(func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })

	r := setupMetricsRouter(t, h)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=00000000000000c1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	if _, ok := resp["error"]; ok {
		t.Fatalf("expected envelope.error omitted for canceled request")
	}
	data := mustMap(t, resp["data"], "data")
	if data["outcome"] != "canceled" {
		t.Fatalf("data.outcome: got=%v want=%q", data["outcome"], "canceled")
	}
	if data["status"] != "canceled" {
		t.Fatalf("data.status: got=%v want=%q", data["status"], "canceled")
	}
	if v, ok := data["error_info"]; !ok || v != nil {
		t.Fatalf("data.error_info: expected null, got=%v (ok=%t)", v, ok)
	}
}

func TestManagementMetrics_PersistenceMetaOmittedWhenNotDegraded(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil, management.WithPersistenceHealthProvider(func(time.Time) metricspersist.PersistenceHealth {
		return metricspersist.PersistenceHealth{Degraded: false}
	}))

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)
	h.SetNowUTC(func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })

	r := setupMetricsRouter(t, h)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=0000000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	meta := mustMap(t, resp["meta"], "meta")
	if _, ok := meta["persistence"]; ok {
		t.Fatalf("expected meta.persistence omitted when not degraded")
	}
}

func TestManagementMetrics_PersistenceMetaMinimalFieldsWhenDegraded(t *testing.T) {
	last := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil, management.WithPersistenceHealthProvider(func(time.Time) metricspersist.PersistenceHealth {
		reason := metricspersist.DropReasonQueueFull
		return metricspersist.PersistenceHealth{
			Degraded:       true,
			DroppedTotal:   7,
			LastDropAt:     last,
			LastDropReason: &reason,
		}
	}))

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)
	h.SetNowUTC(func() time.Time { return time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC) })

	r := setupMetricsRouter(t, h)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=0000000000000001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	meta := mustMap(t, resp["meta"], "meta")
	persist := assertPersistenceMetaMinimalAndSafe(t, meta)

	if persist["degraded"] != true {
		t.Fatalf("persistence.degraded: got=%v want=true", persist["degraded"])
	}
	if persist["dropped_total"] != float64(7) {
		t.Fatalf("persistence.dropped_total: got=%v want=%d", persist["dropped_total"], 7)
	}
	if persist["last_drop_at"] != last.UTC().Format(time.RFC3339) {
		t.Fatalf("persistence.last_drop_at: got=%v want=%s", persist["last_drop_at"], last.UTC().Format(time.RFC3339))
	}

	// Security boundary: persistence meta MUST NOT embed request IDs, SQL errors, or file paths.
	if v, ok := persist["last_drop_reason"]; ok {
		if v != string(metricspersist.DropReasonQueueFull) && v != string(metricspersist.DropReasonWriterNotStarted) && v != string(metricspersist.DropReasonInsertFailure) && v != string(metricspersist.DropReasonRequestIDConflict) {
			t.Fatalf("persistence.last_drop_reason: got=%v expected stable enum", v)
		}
	}
}

func TestManagementMetrics_RequestIDNotFound(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil)

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)

	r := setupMetricsRouter(t, h)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=ffffffffffffffff", nil)
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

	bad := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=0000000000000001&streaming=maybe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, bad)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}

	good := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=0000000000000001&streaming=true", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, good)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	good = httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id=0000000000000001&streaming=1", nil)
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
	if resp.Meta.CanceledCount == nil || *resp.Meta.CanceledCount != 1 {
		t.Fatalf("meta.canceled_count: got=%v want=%d", resp.Meta.CanceledCount, 1)
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
	if s.Metrics.TPSGen.SampleCount != 2 {
		t.Fatalf("success.tps_gen.sample_count: got %d want %d", s.Metrics.TPSGen.SampleCount, 2)
	}
	if s.Metrics.TPSGen.P50 == nil || s.Metrics.TPSGen.P95 == nil || s.Metrics.TPSGen.P99 == nil {
		t.Fatalf("success.tps_gen percentiles expected non-null")
	}
	assertFloatApprox(t, *s.Metrics.TPSGen.P50, 20.0, 1e-9)
	assertFloatApprox(t, *s.Metrics.TPSGen.P95, 29.0, 1e-9)
	assertFloatApprox(t, *s.Metrics.TPSGen.P99, 29.8, 1e-9)

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
	if f.Metrics.TPSGen.SampleCount != 0 {
		t.Fatalf("failure.tps_gen.sample_count: got %d want %d", f.Metrics.TPSGen.SampleCount, 0)
	}
	if f.Metrics.TPSGen.P50 != nil || f.Metrics.TPSGen.P95 != nil || f.Metrics.TPSGen.P99 != nil {
		t.Fatalf("failure.tps_gen percentiles expected null when no samples")
	}
}

func TestManagementMetrics_BucketsMode_AlignmentAndEmptyBuckets(t *testing.T) {
	cfg := &config.Config{}
	h := management.NewHandler(cfg, "", nil)

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	seedBucketsMetricsDB(t, dbPath)
	h.SetMetricsDBPath(dbPath)

	r := setupMetricsRouter(t, h)
	url := "/v0/management/metrics?mode=buckets&bucket=5m&from=2026-01-01T00:02:00Z&to=2026-01-01T00:17:00Z&provider=openai&model=gpt-4o&streaming=true"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp bucketsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Meta.Mode != "buckets" {
		t.Fatalf("meta.mode: got %q want %q", resp.Meta.Mode, "buckets")
	}
	if resp.Meta.Bucket != "5m" {
		t.Fatalf("meta.bucket: got %q want %q", resp.Meta.Bucket, "5m")
	}
	if resp.Meta.Timezone != "UTC" {
		t.Fatalf("meta.timezone: got %q want %q", resp.Meta.Timezone, "UTC")
	}
	if resp.Meta.EffectiveFrom != "2026-01-01T00:00:00Z" {
		t.Fatalf("meta.effective_from: got %q want %q", resp.Meta.EffectiveFrom, "2026-01-01T00:00:00Z")
	}
	if resp.Meta.EffectiveTo != "2026-01-01T00:20:00Z" {
		t.Fatalf("meta.effective_to: got %q want %q", resp.Meta.EffectiveTo, "2026-01-01T00:20:00Z")
	}

	if len(resp.Success) != 1 {
		t.Fatalf("success groups: got %d want %d", len(resp.Success), 1)
	}
	if len(resp.Failure) != 1 {
		t.Fatalf("failure groups: got %d want %d", len(resp.Failure), 1)
	}

	s := resp.Success[0]
	if len(s.Buckets) != 4 {
		t.Fatalf("success buckets length: got %d want %d", len(s.Buckets), 4)
	}
	// Empty bucket contract.
	if s.Buckets[0].Count != 0 {
		t.Fatalf("empty bucket count: got %d want %d", s.Buckets[0].Count, 0)
	}
	if s.Buckets[0].CanceledCount != 0 {
		t.Fatalf("empty bucket canceled_count: got %d want %d", s.Buckets[0].CanceledCount, 0)
	}
	if s.Buckets[0].Metrics.TTFTMillisAvg != nil || s.Buckets[0].Metrics.TPOTMillisAvg != nil || s.Buckets[0].Metrics.DurationMSAvg != nil || s.Buckets[0].Metrics.TPSE2EAvg != nil {
		t.Fatalf("empty bucket metrics expected null")
	}
	if s.Buckets[0].Metrics.TPSGenSampleCount != 0 || s.Buckets[0].Metrics.TPSE2ESampleCount != 0 || s.Buckets[0].Metrics.TTFTMillisSampleCount != 0 || s.Buckets[0].Metrics.TPOTMillisSampleCount != 0 || s.Buckets[0].Metrics.DurationMSSampleCount != 0 {
		t.Fatalf("empty bucket metrics sample_count expected 0")
	}

	// Middle bucket (00:10-00:15) has 1 success row.
	if s.Buckets[2].Start != "2026-01-01T00:10:00Z" {
		t.Fatalf("data bucket start: got %q want %q", s.Buckets[2].Start, "2026-01-01T00:10:00Z")
	}
	if s.Buckets[2].Count != 1 {
		t.Fatalf("data bucket count: got %d want %d", s.Buckets[2].Count, 1)
	}
	if s.Buckets[2].CanceledCount != 1 {
		t.Fatalf("data bucket canceled_count: got %d want %d", s.Buckets[2].CanceledCount, 1)
	}
	if s.Buckets[2].Metrics.TPSGenSampleCount != 1 {
		t.Fatalf("tps_gen_sample_count: got=%d want=%d", s.Buckets[2].Metrics.TPSGenSampleCount, 1)
	}
	if s.Buckets[2].Metrics.TPSE2ESampleCount != 1 {
		t.Fatalf("tps_e2e_sample_count: got=%d want=%d", s.Buckets[2].Metrics.TPSE2ESampleCount, 1)
	}
	if s.Buckets[2].Metrics.TTFTMillisSampleCount != 1 {
		t.Fatalf("ttft_ms_sample_count: got=%d want=%d", s.Buckets[2].Metrics.TTFTMillisSampleCount, 1)
	}
	if s.Buckets[2].Metrics.TPOTMillisSampleCount != 1 {
		t.Fatalf("tpot_ms_sample_count: got=%d want=%d", s.Buckets[2].Metrics.TPOTMillisSampleCount, 1)
	}
	if s.Buckets[2].Metrics.DurationMSSampleCount != 1 {
		t.Fatalf("duration_ms_sample_count: got=%d want=%d", s.Buckets[2].Metrics.DurationMSSampleCount, 1)
	}
	if s.Buckets[2].Metrics.TTFTMillisAvg == nil || *s.Buckets[2].Metrics.TTFTMillisAvg != 2 {
		t.Fatalf("ttft_ms_avg: got=%v want=%d", s.Buckets[2].Metrics.TTFTMillisAvg, 2)
	}
	if s.Buckets[2].Metrics.TPSE2EAvg == nil {
		t.Fatalf("tps_e2e_avg: expected non-null")
	}
	assertFloatApprox(t, *s.Buckets[2].Metrics.TPSE2EAvg, 20.0/(1234.0/1000.0), 1e-9)
	if s.Buckets[2].Metrics.DurationMSAvg == nil || *s.Buckets[2].Metrics.DurationMSAvg != 1234 {
		t.Fatalf("duration_ms_avg: got=%v want=%d", s.Buckets[2].Metrics.DurationMSAvg, 1234)
	}

	f := resp.Failure[0]
	if len(f.Buckets) != 4 {
		t.Fatalf("failure buckets length: got %d want %d", len(f.Buckets), 4)
	}
	if f.Buckets[0].Count != 0 {
		t.Fatalf("failure empty bucket count: got %d want %d", f.Buckets[0].Count, 0)
	}
	if f.Buckets[0].CanceledCount != 0 {
		t.Fatalf("failure empty bucket canceled_count: got %d want %d", f.Buckets[0].CanceledCount, 0)
	}
	if f.Buckets[0].Metrics.TTFTMillisAvg != nil || f.Buckets[0].Metrics.TPOTMillisAvg != nil || f.Buckets[0].Metrics.DurationMSAvg != nil || f.Buckets[0].Metrics.TPSE2EAvg != nil {
		t.Fatalf("failure empty bucket metrics expected null")
	}
	if f.Buckets[0].Metrics.TPSGenSampleCount != 0 || f.Buckets[0].Metrics.TPSE2ESampleCount != 0 || f.Buckets[0].Metrics.TTFTMillisSampleCount != 0 || f.Buckets[0].Metrics.TPOTMillisSampleCount != 0 || f.Buckets[0].Metrics.DurationMSSampleCount != 0 {
		t.Fatalf("failure empty bucket metrics sample_count expected 0")
	}

	// Middle bucket (00:10-00:15) has 2 failure rows: one non-2xx status and one 200+error_info streaming failure.
	if f.Buckets[2].Start != "2026-01-01T00:10:00Z" {
		t.Fatalf("failure data bucket start: got %q want %q", f.Buckets[2].Start, "2026-01-01T00:10:00Z")
	}
	if f.Buckets[2].Count != 2 {
		t.Fatalf("failure data bucket count: got %d want %d", f.Buckets[2].Count, 2)
	}
	if f.Buckets[2].CanceledCount != 1 {
		t.Fatalf("failure data bucket canceled_count: got %d want %d", f.Buckets[2].CanceledCount, 1)
	}
	if f.Buckets[2].Metrics.TPSGenSampleCount != 0 {
		t.Fatalf("failure tps_gen_sample_count: got=%d want=%d", f.Buckets[2].Metrics.TPSGenSampleCount, 0)
	}
	if f.Buckets[2].Metrics.TPSE2ESampleCount != 0 {
		t.Fatalf("failure tps_e2e_sample_count: got=%d want=%d", f.Buckets[2].Metrics.TPSE2ESampleCount, 0)
	}
	if f.Buckets[2].Metrics.TPSE2EAvg != nil {
		t.Fatalf("failure tps_e2e_avg: expected null")
	}
	if f.Buckets[2].Metrics.TTFTMillisSampleCount != 1 {
		t.Fatalf("failure ttft_ms_sample_count: got=%d want=%d", f.Buckets[2].Metrics.TTFTMillisSampleCount, 1)
	}
	if f.Buckets[2].Metrics.TPOTMillisSampleCount != 0 {
		t.Fatalf("failure tpot_ms_sample_count: got=%d want=%d", f.Buckets[2].Metrics.TPOTMillisSampleCount, 0)
	}
	if f.Buckets[2].Metrics.DurationMSSampleCount != 1 {
		t.Fatalf("failure duration_ms_sample_count: got=%d want=%d", f.Buckets[2].Metrics.DurationMSSampleCount, 1)
	}
	total := s.Buckets[2].Count + f.Buckets[2].Count + s.Buckets[2].CanceledCount
	if total != 4 {
		t.Fatalf("total count mismatch: got=%d want=%d", total, 4)
	}
}
