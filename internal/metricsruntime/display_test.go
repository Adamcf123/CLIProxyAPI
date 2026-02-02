package metricsruntime

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	_ = w.Close()
	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("read stderr pipe: %v", readErr)
	}
	return string(out)
}

func TestPrintProgress(t *testing.T) {
	t.Run("non_tty_silent", func(t *testing.T) {
		out := captureStderr(t, func() {
			state := NewRequestState(true, "test-model")
			PrintProgress(state, false)
		})
		if out != "" {
			t.Fatalf("expected no output on non-TTY stderr, got %q", out)
		}
	})

	t.Run("tty_but_env_disabled_silent_progress_and_summary_preserved", func(t *testing.T) {
		t.Setenv(EnvMetricsProgressDisabled, "1")
		out := captureStderr(t, func() {
			state := NewRequestState(true, "test-model")
			PrintProgress(state, true)
			PrintSummary(state)
		})

		if strings.Contains(out, "\r") {
			t.Fatalf("expected no overwrite progress (\\r), got %q", out)
		}
		if strings.Contains(out, "\033[2K") {
			t.Fatalf("expected no ANSI clear progress, got %q", out)
		}
		if strings.Contains(out, "metrics tracking=") {
			t.Fatalf("expected no progress text, got %q", out)
		}
		if strings.Count(out, "\n") != 1 {
			t.Fatalf("expected a single summary line, got %q", out)
		}

		line := strings.TrimSpace(out)
		if !strings.HasPrefix(line, "metrics_summary ") {
			t.Fatalf("expected metrics_summary line, got %q", line)
		}

		payload := strings.TrimPrefix(line, "metrics_summary ")
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("expected metrics_summary JSON, unmarshal error: %v payload=%q", err, payload)
		}
		if _, ok := m["window_stats"].(map[string]any); !ok {
			t.Fatalf("expected window_stats object in metrics_summary, got %T", m["window_stats"])
		}
		if _, ok := m["errors_total"].(float64); !ok {
			t.Fatalf("expected errors_total number in metrics_summary, got %T", m["errors_total"])
		}
	})

	t.Run("tty_but_config_disabled_silent_progress_and_summary_preserved", func(t *testing.T) {
		old := ProgressDisabled()
		SetProgressDisabled(true)
		t.Cleanup(func() { SetProgressDisabled(old) })
		out := captureStderr(t, func() {
			state := NewRequestState(true, "test-model")
			PrintProgress(state, true)
			PrintSummary(state)
		})

		if strings.Contains(out, "\r") {
			t.Fatalf("expected no overwrite progress (\\r), got %q", out)
		}
		if strings.Contains(out, "\033[2K") {
			t.Fatalf("expected no ANSI clear progress, got %q", out)
		}
		if strings.Contains(out, "metrics tracking=") {
			t.Fatalf("expected no progress text, got %q", out)
		}
		if strings.Count(out, "\n") != 1 {
			t.Fatalf("expected a single summary line, got %q", out)
		}
	})

	t.Run("stop_emits_summary_even_when_non_tty", func(t *testing.T) {
		out := captureStderr(t, func() {
			state := NewRequestState(true, "test-model")
			stop := StartLiveDisplay(state)
			stop()
		})

		if strings.Contains(out, "metrics tracking=") {
			t.Fatalf("expected no progress lines, got %q", out)
		}
		if strings.Count(out, "\n") != 1 {
			t.Fatalf("expected a single summary line, got %q", out)
		}

		line := strings.TrimSpace(out)
		if !strings.HasPrefix(line, "metrics_summary ") {
			t.Fatalf("expected metrics_summary line, got %q", line)
		}

		payload := strings.TrimPrefix(line, "metrics_summary ")
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("expected metrics_summary JSON, unmarshal error: %v payload=%q", err, payload)
		}

		trackingID, _ := m["tracking_id"].(string)
		if strings.TrimSpace(trackingID) == "" {
			t.Fatalf("expected non-empty tracking_id in metrics_summary, got %v", m["tracking_id"])
		}
		if _, ok := m["window_stats"].(map[string]any); !ok {
			t.Fatalf("expected window_stats object in metrics_summary, got %T", m["window_stats"])
		}
		if _, ok := m["errors_total"].(float64); !ok {
			t.Fatalf("expected errors_total number in metrics_summary, got %T", m["errors_total"])
		}
	})
}

func TestPrintSummary_WindowStats_EncodesNullsAndNumbers(t *testing.T) {
	t.Run("empty window -> avg null", func(t *testing.T) {
		out := captureStderr(t, func() {
			state := NewRequestState(true, "test-model")
			PrintSummary(state)
		})

		line := strings.TrimSpace(out)
		if !strings.HasPrefix(line, "metrics_summary ") {
			t.Fatalf("expected metrics_summary line, got %q", line)
		}
		payload := strings.TrimPrefix(line, "metrics_summary ")
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("expected metrics_summary JSON, unmarshal error: %v payload=%q", err, payload)
		}
		ws, ok := m["window_stats"].(map[string]any)
		if !ok {
			t.Fatalf("expected window_stats object, got %T", m["window_stats"])
		}
		if got, ok := ws["count"].(float64); !ok || got != 0 {
			t.Fatalf("expected window_stats.count=0, got %v (%T)", ws["count"], ws["count"])
		}
		if ws["tps_avg"] != nil || ws["ttft_avg"] != nil || ws["tpot_avg"] != nil {
			t.Fatalf("expected window_stats avg fields to be null for empty window, got tps_avg=%v ttft_avg=%v tpot_avg=%v", ws["tps_avg"], ws["ttft_avg"], ws["tpot_avg"])
		}
	})

	t.Run("non-empty window -> avg numbers", func(t *testing.T) {
		out := captureStderr(t, func() {
			state := NewRequestState(true, "test-model")
			tps := 12.34
			ttft := 0.56
			tpot := 0.078
			state.SetWindowStats(RequestWindowStats{Count: 3, TPSAvg: &tps, TTFTAvg: &ttft, TPOTAvg: &tpot})
			PrintSummary(state)
		})

		line := strings.TrimSpace(out)
		if !strings.HasPrefix(line, "metrics_summary ") {
			t.Fatalf("expected metrics_summary line, got %q", line)
		}
		payload := strings.TrimPrefix(line, "metrics_summary ")
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("expected metrics_summary JSON, unmarshal error: %v payload=%q", err, payload)
		}
		ws, ok := m["window_stats"].(map[string]any)
		if !ok {
			t.Fatalf("expected window_stats object, got %T", m["window_stats"])
		}
		if got, ok := ws["count"].(float64); !ok || got < 1 {
			t.Fatalf("expected window_stats.count>=1, got %v (%T)", ws["count"], ws["count"])
		}
		if _, ok := ws["tps_avg"].(float64); !ok {
			t.Fatalf("expected window_stats.tps_avg number, got %v (%T)", ws["tps_avg"], ws["tps_avg"])
		}
		if _, ok := ws["ttft_avg"].(float64); !ok {
			t.Fatalf("expected window_stats.ttft_avg number, got %v (%T)", ws["ttft_avg"], ws["ttft_avg"])
		}
		if _, ok := ws["tpot_avg"].(float64); !ok {
			t.Fatalf("expected window_stats.tpot_avg number, got %v (%T)", ws["tpot_avg"], ws["tpot_avg"])
		}
	})
}

func TestMetricsSummary_ErrorsTotal_Semantics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	p := NewMetricsPlugin(nil)
	record := usage.Record{
		Provider: "openai",
		Model:    "gpt-5.2",
		Detail: usage.Detail{
			OutputTokens: 100,
			TotalTokens:  100,
		},
	}

	run := func(t *testing.T, name string, setState func(*RequestState)) float64 {
		t.Helper()
		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		state := NewRequestState(false, "gpt-5.2")
		state.StartedAt = time.Now().Add(-1 * time.Second)
		setState(state)
		AttachRequestState(ginCtx, state)

		ctx := context.WithValue(context.Background(), "gin", ginCtx)
		p.HandleUsage(ctx, record)

		out := captureStderr(t, func() { PrintSummary(state) })
		line := strings.TrimSpace(out)
		if !strings.HasPrefix(line, "metrics_summary ") {
			t.Fatalf("%s: expected metrics_summary line, got %q", name, line)
		}
		payload := strings.TrimPrefix(line, "metrics_summary ")
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("%s: expected metrics_summary JSON, unmarshal error: %v payload=%q", name, err, payload)
		}
		v, ok := m["errors_total"].(float64)
		if !ok {
			t.Fatalf("%s: expected errors_total number, got %v (%T)", name, m["errors_total"], m["errors_total"])
		}
		return v
	}

	if got := run(t, "success", func(s *RequestState) { s.SetStatusCode(200) }); got != 0 {
		t.Fatalf("expected errors_total=0 after success, got %v", got)
	}
	if got := run(t, "failure", func(s *RequestState) { s.SetStatusCode(500) }); got != 1 {
		t.Fatalf("expected errors_total=1 after failure, got %v", got)
	}
	if got := run(t, "success again", func(s *RequestState) { s.SetStatusCode(200) }); got != 1 {
		t.Fatalf("expected errors_total to remain 1 after success, got %v", got)
	}
	if got := run(t, "canceled=499", func(s *RequestState) {
		s.SetStatusCode(200)
		s.MarkClientCanceled()
	}); got != 1 {
		t.Fatalf("expected errors_total to remain 1 after canceled, got %v", got)
	}
}
