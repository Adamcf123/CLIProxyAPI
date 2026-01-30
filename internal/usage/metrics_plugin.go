package usage

import (
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricsruntime"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func init() {
	// Register MetricsPlugin alongside LoggerPlugin to compute TPS/TTFT/TPOT
	// metrics and persist them via the SQLite metrics database (default: logs/metrics.db).
	collector := metrics.NewCollector(nil)
	coreusage.RegisterPlugin(metricsruntime.NewMetricsPlugin(collector))
}
