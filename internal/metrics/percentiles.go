package metrics

import "sort"

// CalculatePercentile computes the p-th percentile from a sorted slice.
//
// Semantics are intentionally identical to the existing linear interpolation
// implementation in internal/metrics/window.go (calculatePercentile).
//
// Note: For an empty slice, it returns 0 to match the underlying implementation.
func CalculatePercentile(sorted []float64, p float64) float64 {
	return calculatePercentile(sorted, p)
}

// CalculateP50P95P99 returns p50/p95/p99 for the given values.
// It copies and sorts the input to avoid mutating the caller's slice.
// If values is empty, ok is false.
func CalculateP50P95P99(values []float64) (p50, p95, p99 float64, ok bool) {
	if len(values) == 0 {
		return 0, 0, 0, false
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	return CalculatePercentile(sorted, 50), CalculatePercentile(sorted, 95), CalculatePercentile(sorted, 99), true
}
