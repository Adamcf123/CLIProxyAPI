package metrics

import (
	"math"
	"testing"
	"time"
)

func nearlyEqual(a float64, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestE2ECollector_UsesFixedTenMinuteWindow(t *testing.T) {
	base := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	now := base
	collector := newE2ECollectorWithClock(func() time.Time { return now })
	key := MetricKey{Provider: "openai", Model: "gpt-5", Streaming: true}

	collector.Add(key, 10, 0.1)
	now = base.Add(5 * time.Minute)
	collector.Add(key, 20, 0.05)

	count, tpsAvg, tpotAvg, ok := collector.GetAverages(key)
	if !ok {
		t.Fatalf("expected averages to exist")
	}
	if count != 2 {
		t.Fatalf("expected count=2, got %d", count)
	}
	if tpsAvg != 15 {
		t.Fatalf("expected tps average=15, got %v", tpsAvg)
	}
	if !nearlyEqual(tpotAvg, 0.075) {
		t.Fatalf("expected tpot average=0.075, got %v", tpotAvg)
	}

	now = base.Add(11 * time.Minute)
	count, tpsAvg, tpotAvg, ok = collector.GetAverages(key)
	if !ok {
		t.Fatalf("expected averages to still exist after first sample expires")
	}
	if count != 1 {
		t.Fatalf("expected count=1 after expiration, got %d", count)
	}
	if tpsAvg != 20 {
		t.Fatalf("expected tps average=20 after expiration, got %v", tpsAvg)
	}
	if !nearlyEqual(tpotAvg, 0.05) {
		t.Fatalf("expected tpot average=0.05 after expiration, got %v", tpotAvg)
	}

	now = base.Add(16 * time.Minute)
	count, tpsAvg, tpotAvg, ok = collector.GetAverages(key)
	if ok {
		t.Fatalf("expected no averages after all samples expire, got count=%d tps=%v tpot=%v", count, tpsAvg, tpotAvg)
	}
}
