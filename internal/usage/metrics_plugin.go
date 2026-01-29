package usage

import (
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricsruntime"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func init() {
	// Register MetricsPlugin alongside LoggerPlugin to compute and persist
	// TPS/TTFT/TPOT metrics as structured JSONL logs.
	collector := metrics.NewCollector(nil)
	coreusage.RegisterPlugin(metricsruntime.NewMetricsPlugin(collector))
}
