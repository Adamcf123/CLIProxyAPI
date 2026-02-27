package metricsruntime

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
)

const EnvMetricsProgressDisabled = "CLIPROXY_METRICS_PROGRESS_DISABLED"

var configProgressDisabled atomic.Bool

// SetProgressDisabled is kept for backward compatibility.
// Real-time progress output is removed and this flag has no runtime effect.
func SetProgressDisabled(disabled bool) {
	configProgressDisabled.Store(disabled)
}

func ProgressDisabled() bool {
	return configProgressDisabled.Load()
}

// StartLiveDisplay returns an idempotent stop function for final metrics output.
// Real-time progress output has been removed; stop only prints a final summary once.
func StartLiveDisplay(state *RequestState) (stop func()) {
	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			PrintSummary(state)
		})
	}
}

func PrintProgress(state *RequestState, isTTY bool) {
	// Real-time progress logging has been removed.
	_ = state
	_ = isTTY
}

func formatSummaryTPSAvg(stats RequestWindowStatsE2E) string {
	if stats.TPSE2EAvg == nil {
		return "--"
	}
	v := *stats.TPSE2EAvg
	if v <= 0 {
		return "--"
	}
	return fmt.Sprintf("%.3f", v)
}

func PrintSummary(state *RequestState) {
	if state == nil {
		return
	}
	snap := state.Snapshot()

	_, _ = fmt.Fprintf(os.Stderr, "metrics_summary provider=%s\n", snap.Provider)
	_, _ = fmt.Fprintf(os.Stderr, "metrics_summary model=%s\n", snap.Model)

	windowStatsE2E := snap.WindowStatsE2E
	_, _ = fmt.Fprintf(os.Stderr, "metrics_summary request_count=%d\n", windowStatsE2E.Count)
	_, _ = fmt.Fprintf(os.Stderr, "metrics_summary time_window=%s\n", metrics.E2EWindowLabel)
	_, _ = fmt.Fprintf(os.Stderr, "metrics_summary tps_avg=%s\n", formatSummaryTPSAvg(windowStatsE2E))
}
