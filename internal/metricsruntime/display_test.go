package metricsruntime

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
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
	})
}
