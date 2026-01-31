package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricspersist"
)

func TestGetMetrics_OmitsPersistenceMetaWhenNotDegraded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	h := NewHandler(nil, "", nil, WithPersistenceHealthProvider(func(time.Time) metricspersist.PersistenceHealth {
		return metricspersist.PersistenceHealth{Degraded: false}
	}))
	h.SetNowUTC(func() time.Time { return fixedNow })
	h.SetMetricsDBPath(filepath.Join(t.TempDir(), "metrics.db"))

	r := gin.New()
	r.GET("/v0/management/metrics", h.GetMetrics)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?mode=percentiles", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	meta, ok := body["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta: missing or wrong type")
	}
	if _, ok := meta["persistence"]; ok {
		t.Fatalf("expected meta.persistence omitted when not degraded")
	}
}

func TestGetMetrics_EmitsPersistenceMetaOnlyWhenDegraded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	last := fixedNow.Add(-30 * time.Second)
	reason := metricspersist.DropReasonQueueFull

	h := NewHandler(nil, "", nil, WithPersistenceHealthProvider(func(time.Time) metricspersist.PersistenceHealth {
		return metricspersist.PersistenceHealth{
			Degraded:       true,
			DroppedTotal:   7,
			LastDropAt:     last,
			LastDropReason: &reason,
		}
	}))
	h.SetNowUTC(func() time.Time { return fixedNow })
	h.SetMetricsDBPath(filepath.Join(t.TempDir(), "metrics.db"))

	r := gin.New()
	r.GET("/v0/management/metrics", h.GetMetrics)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?mode=percentiles", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	meta, ok := body["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta: missing or wrong type")
	}
	persist, ok := meta["persistence"].(map[string]any)
	if !ok {
		t.Fatalf("meta.persistence: missing or wrong type")
	}

	// Ensure the public contract stays minimal and stable.
	if got, ok := persist["degraded"].(bool); !ok || !got {
		t.Fatalf("persistence.degraded: got=%v want=true", persist["degraded"])
	}
	if got, ok := persist["dropped_total"].(float64); !ok || got != 7 {
		t.Fatalf("persistence.dropped_total: got=%v want=7", persist["dropped_total"])
	}
	if got, ok := persist["last_drop_at"].(string); !ok || got != last.UTC().Format(time.RFC3339) {
		t.Fatalf("persistence.last_drop_at: got=%v want=%s", persist["last_drop_at"], last.UTC().Format(time.RFC3339))
	}
	if got, ok := persist["last_drop_reason"].(string); !ok || got != string(metricspersist.DropReasonQueueFull) {
		t.Fatalf("persistence.last_drop_reason: got=%v want=%s", persist["last_drop_reason"], metricspersist.DropReasonQueueFull)
	}
	if len(persist) != 4 {
		t.Fatalf("persistence meta leaked extra fields: keys=%v", persist)
	}
}

func TestGetMetrics_ExposesRequestIDConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Use a real writer + DB to lock the full chain:
	// conflict detection -> PersistenceHealth -> meta.persistence exposure.
	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	db, err := metricspersist.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	// Keep the DB open for the whole test so the writer doesn't record insert failures.
	t.Cleanup(func() { _ = db.Close() })
	if err := metricspersist.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := metricspersist.StartWriter(db); err != nil {
		t.Fatalf("StartWriter: %v", err)
	}

	requestID := "conflict_test_001"

	// Ensure a row already exists so the writer insert hits ON CONFLICT DO NOTHING.
	if _, err := db.Exec(
		`INSERT INTO metrics (request_id, provider, model, created_at)
		 VALUES (?, ?, ?, datetime('now'));`,
		requestID,
		"test",
		"model",
	); err != nil {
		t.Fatalf("insert seed row: %v", err)
	}

	baseline := metricspersist.GetPersistenceHealth(time.Now().UTC()).DroppedTotal

	metricspersist.Enqueue(metricspersist.MetricRecord{
		RequestID: requestID,
		Provider:  "test",
		Model:     "model",
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		h := metricspersist.GetPersistenceHealth(time.Now().UTC())
		if h.DroppedTotal >= baseline+1 && h.LastDropReason != nil && *h.LastDropReason == metricspersist.DropReasonRequestIDConflict {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for request_id conflict: DroppedTotal=%d LastDropReason=%v", h.DroppedTotal, h.LastDropReason)
		}
		time.Sleep(20 * time.Millisecond)
	}

	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	h := NewHandler(nil, "", nil)
	h.SetNowUTC(func() time.Time { return fixedNow })
	h.SetMetricsDBPath(dbPath)

	r := gin.New()
	r.GET("/v0/management/metrics", h.GetMetrics)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/management/metrics?request_id="+requestID, nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	meta, ok := body["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta: missing or wrong type")
	}
	persist, ok := meta["persistence"].(map[string]any)
	if !ok {
		t.Fatalf("meta.persistence: missing or wrong type")
	}

	if got, ok := persist["degraded"].(bool); !ok || !got {
		t.Fatalf("persistence.degraded: got=%v want=true", persist["degraded"])
	}
	if got, ok := persist["last_drop_reason"].(string); !ok || got != string(metricspersist.DropReasonRequestIDConflict) {
		t.Fatalf("persistence.last_drop_reason: got=%v want=%s", persist["last_drop_reason"], metricspersist.DropReasonRequestIDConflict)
	}
}
