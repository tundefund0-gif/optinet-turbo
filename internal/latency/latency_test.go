package latency

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestNewTester(t *testing.T) {
	tester := NewTester()
	if tester == nil {
		t.Fatal("NewTester returned nil")
	}
	if tester.timeout <= 0 {
		t.Error("Timeout should be positive")
	}
	if tester.sampleSize <= 0 {
		t.Error("SampleSize should be positive")
	}
}

func TestPingTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test")
	}

	tester := NewTester()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sample, err := tester.PingTarget(ctx, "8.8.8.8:443")
	if err != nil {
		t.Fatalf("PingTarget failed: %v", err)
	}
	if !sample.Success {
		t.Error("Expected successful ping to 8.8.8.8:443")
	}
	if sample.Latency <= 0 {
		t.Error("Expected positive latency")
	}
}

func TestBenchmarkTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test")
	}

	tester := NewTester()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	stats, err := tester.BenchmarkTarget(ctx, "8.8.8.8:443", 3)
	if err != nil {
		t.Fatalf("BenchmarkTarget failed: %v", err)
	}
	
	if stats.Samples != 3 {
		t.Errorf("Expected 3 samples, got %d", stats.Samples)
	}
	if stats.Loss > 100 {
		t.Errorf("Loss > 100%%: %.2f", stats.Loss)
	}
	if stats.Avg <= 0 && stats.Lost != stats.Samples {
		t.Error("Expected positive average latency for reachable targets")
	}
}

func TestBenchmarkTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test")
	}

	tester := NewTester()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	targets := []string{"8.8.8.8:443", "1.1.1.1:443"}
	results, err := tester.BenchmarkTargets(ctx, targets)
	if err != nil {
		t.Fatalf("BenchmarkTargets failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("No results returned")
	}

	// Should be sorted by latency
	for i := 1; i < len(results); i++ {
		if results[i].Stats.Avg < results[i-1].Stats.Avg {
			t.Error("Results not sorted by average latency")
		}
	}
}

func TestStatsComputation(t *testing.T) {
	stats := computeStats(
		[]float64{10000, 20000, 15000, 30000, 12000}, // 10-30ms in microseconds
		5, 0, 5,
	)

	if stats.Min <= 0 {
		t.Error("Min should be positive")
	}
	if stats.Max <= stats.Min {
		t.Error("Max should be > Min")
	}
	if stats.Avg <= 0 {
		t.Error("Avg should be positive")
	}
	if stats.P95 <= stats.Median {
		t.Error("P95 should be >= P50")
	}
	if stats.Loss != 0 {
		t.Errorf("Expected 0%% loss, got %.1f%%", stats.Loss)
	}
}

func TestLossDetection(t *testing.T) {
	stats := computeStats(
		[]float64{10000},
		1, 4, 5,
	)

	if stats.Loss != 80.0 {
		t.Errorf("Expected 80%% loss, got %.1f%%", stats.Loss)
	}
	if stats.Lost != 4 {
		t.Errorf("Expected 4 lost, got %d", stats.Lost)
	}
}

func TestGameTargets(t *testing.T) {
	targets := GameTargets()
	if len(targets) == 0 {
		t.Fatal("GameTargets returned empty")
	}
	for _, tgt := range targets {
		if tgt == "" {
			t.Error("Empty target in GameTargets")
		}
	}
}

func TestFindFastestTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test")
	}

	tester := NewTester()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	targets := []string{"8.8.8.8:443", "1.1.1.1:443"}
	result, err := tester.FindFastestTarget(ctx, targets)
	if err != nil {
		t.Fatalf("FindFastestTarget failed: %v", err)
	}

	if !result.Reachable {
		t.Error("Expected at least one reachable target")
	}
}

func TestPercentileCalculation(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	tests := []struct {
		p    float64
		want float64
	}{
		{0, 1},
		{50, 5.5},
		{100, 10},
		{25, 3.25},
		{75, 7.75},
	}

	for _, tt := range tests {
		got := percentile(data, tt.p)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("percentile(%f) = %f, want %f", tt.p, got, tt.want)
		}
	}
}

func BenchmarkPingTarget(b *testing.B) {
	tester := NewTester()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tester.PingTarget(ctx, "8.8.8.8:443")
	}
}

func BenchmarkBenchmarkTargets(b *testing.B) {
	tester := NewTester()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	targets := []string{"8.8.8.8:443", "1.1.1.1:443"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tester.BenchmarkTargets(ctx, targets)
	}
}
