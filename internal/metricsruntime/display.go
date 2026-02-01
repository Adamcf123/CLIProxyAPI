package metricsruntime

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	isatty "github.com/mattn/go-isatty"
)

const EnvMetricsProgressDisabled = "CLIPROXY_METRICS_PROGRESS_DISABLED"

type liveDisplay struct {
	state *RequestState
	isTTY bool

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
	ticker   *time.Ticker
}

// StartLiveDisplay starts a side-channel progress display for a request.
// It only writes to stderr and never touches HTTP response bodies.
// The returned stop function is idempotent and prints a final summary once.
func StartLiveDisplay(state *RequestState) (stop func()) {
	d := &liveDisplay{
		state:  state,
		isTTY:  isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
		ticker: time.NewTicker(1 * time.Second),
	}
	go d.loop()
	return d.stop
}

func (d *liveDisplay) stop() {
	if d == nil {
		return
	}
	d.stopOnce.Do(func() {
		close(d.stopCh)
	})
	<-d.doneCh
}

func (d *liveDisplay) loop() {
	defer close(d.doneCh)
	defer d.ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			// Ensure we leave overwrite mode cleanly.
			if d.isTTY {
				_, _ = fmt.Fprint(os.Stderr, "\r\033[2K\n")
			}
			PrintSummary(d.state)
			return
		case <-d.ticker.C:
			PrintProgress(d.state, d.isTTY)
		}
	}
}

func PrintProgress(state *RequestState, isTTY bool) {
	if state == nil {
		return
	}
	if !isTTY {
		return
	}
	if envBoolTrue(EnvMetricsProgressDisabled) {
		return
	}
	snap := state.Snapshot()

	trackingShort := shortTrackingID(snap.TrackingID)
	elapsed := time.Since(snap.StartedAt).Truncate(100 * time.Millisecond)

	provider := valueOrDash(snap.Provider)
	model := valueOrDash(snap.Model)

	ttft := "--"
	if snap.FirstContentTokenAt != nil {
		secs := snap.FirstContentTokenAt.Sub(snap.StartedAt).Seconds()
		ttft = fmt.Sprintf("%.3fs", secs)
	}

	outputTokens := "--"
	if snap.OutputTokens != nil {
		outputTokens = fmt.Sprintf("%d", *snap.OutputTokens)
	}

	tps := "--"
	if snap.Metrics != nil {
		if snap.Metrics.TPS > 0 {
			tps = fmt.Sprintf("%.2f", snap.Metrics.TPS)
		}
	}

	line := fmt.Sprintf("metrics tracking=%s elapsed=%s ttft=%s tps=%s out=%s provider=%s model=%s",
		trackingShort,
		elapsed,
		ttft,
		tps,
		outputTokens,
		provider,
		model,
	)

	_, _ = fmt.Fprintf(os.Stderr, "\r\033[2K%s", line)
}

func envBoolTrue(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

type summaryLine struct {
	TrackingID   string   `json:"tracking_id"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	TPS          *float64 `json:"tps"`
	TTFT         *float64 `json:"ttft"`
	TPOT         *float64 `json:"tpot"`
	InputTokens  *int     `json:"input_tokens"`
	OutputTokens *int     `json:"output_tokens"`
	DurationMs   int64    `json:"duration_ms"`
	StatusCode   *int     `json:"status_code"`
	RequestPath  string   `json:"request_path"`
	UsageNote    string   `json:"usage_note"`
}

func PrintSummary(state *RequestState) {
	if state == nil {
		return
	}
	snap := state.Snapshot()

	durationMs := time.Since(snap.StartedAt).Milliseconds()

	var tpsPtr, ttftPtr, tpotPtr *float64
	if snap.Metrics != nil {
		if snap.Metrics.TPS > 0 {
			v := snap.Metrics.TPS
			tpsPtr = &v
		}
		if snap.Metrics.TTFT > 0 {
			v := snap.Metrics.TTFT
			ttftPtr = &v
		}
		if snap.Metrics.TPOT > 0 {
			v := snap.Metrics.TPOT
			tpotPtr = &v
		}
	}
	// TTFT is meaningful even when token usage is unavailable; fall back to first-content-token timing.
	if ttftPtr == nil && snap.FirstContentTokenAt != nil {
		secs := snap.FirstContentTokenAt.Sub(snap.StartedAt).Seconds()
		if secs > 0 {
			v := secs
			ttftPtr = &v
		}
	}
	// Non-streaming: "first content token" is only observable at end of response.
	// Report TTFT as total request latency if no other TTFT is available.
	if ttftPtr == nil && !snap.Streaming {
		secs := float64(durationMs) / 1000
		if secs > 0 {
			v := secs
			ttftPtr = &v
		}
	}

	var statusCodePtr *int
	if snap.StatusCode != 0 {
		v := snap.StatusCode
		statusCodePtr = &v
	}

	usageNote := "ok"
	if snap.InputTokens == nil || snap.OutputTokens == nil {
		usageNote = "usage_missing_tokens_unavailable"
	}

	line := summaryLine{
		TrackingID:   snap.TrackingID,
		Provider:     snap.Provider,
		Model:        snap.Model,
		TPS:          tpsPtr,
		TTFT:         ttftPtr,
		TPOT:         tpotPtr,
		InputTokens:  snap.InputTokens,
		OutputTokens: snap.OutputTokens,
		DurationMs:   durationMs,
		StatusCode:   statusCodePtr,
		RequestPath:  snap.RequestPath,
		UsageNote:    usageNote,
	}

	b, err := json.Marshal(line)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "metrics_summary tracking_id=%s marshal_error=%q\n", snap.TrackingID, err.Error())
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "metrics_summary %s\n", string(b))
}

func shortTrackingID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "--"
	}
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func valueOrDash(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "--"
	}
	return s
}
