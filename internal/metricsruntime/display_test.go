package metricsruntime

import (
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

func parseSummaryLines(t *testing.T, out string) map[string]string {
	t.Helper()

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Fatalf("expected summary output, got empty")
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 summary lines, got %d: %q", len(lines), trimmed)
	}

	parsed := make(map[string]string, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(line, "metrics_summary ") {
			t.Fatalf("expected metrics_summary line prefix, got %q", line)
		}
		kv := strings.TrimPrefix(line, "metrics_summary ")
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("expected key=value summary line, got %q", line)
		}
		parsed[parts[0]] = parts[1]
	}

	return parsed
}

func TestPrintProgress_IsSilent(t *testing.T) {
	out := captureStderr(t, func() {
		state := NewRequestState(true, "test-model")
		PrintProgress(state, true)
	})
	if out != "" {
		t.Fatalf("expected no progress output, got %q", out)
	}
}

func TestStartLiveDisplay_StopPrintsSummaryLines(t *testing.T) {
	out := captureStderr(t, func() {
		state := NewRequestState(true, "test-model")
		stop := StartLiveDisplay(state)
		stop()
	})

	parsed := parseSummaryLines(t, out)
	if parsed["request_count"] != "0" {
		t.Fatalf("expected request_count=0, got %q", parsed["request_count"])
	}
	if parsed["time_window"] != "10m" {
		t.Fatalf("expected time_window=10m, got %q", parsed["time_window"])
	}
	if parsed["tps_avg"] != "--" {
		t.Fatalf("expected tps_avg=--, got %q", parsed["tps_avg"])
	}
}

func TestPrintSummary_UsesE2EWindowStatsOnly(t *testing.T) {
	out := captureStderr(t, func() {
		state := NewRequestState(true, "test-model")
		tpsAvg := 23.77743046887295
		state.SetWindowStatsE2E(RequestWindowStatsE2E{Count: 31, TPSE2EAvg: &tpsAvg})
		PrintSummary(state)
	})

	parsed := parseSummaryLines(t, out)
	if parsed["request_count"] != "31" {
		t.Fatalf("expected request_count=31, got %q", parsed["request_count"])
	}
	if parsed["time_window"] != "10m" {
		t.Fatalf("expected time_window=10m, got %q", parsed["time_window"])
	}
	if parsed["tps_avg"] != "23.777" {
		t.Fatalf("expected tps_avg=23.777, got %q", parsed["tps_avg"])
	}
}
