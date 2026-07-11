package benchmark

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestNewSuite(t *testing.T) {
	suite := NewSuite()
	if suite == nil {
		t.Fatal("NewSuite returned nil")
	}
	if suite.dnsOpt == nil {
		t.Error("DNS optimizer not initialized")
	}
	if suite.latTester == nil {
		t.Error("Latency tester not initialized")
	}
	if suite.speedTester == nil {
		t.Error("Speed tester not initialized")
	}
}

func TestQuickBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := QuickBenchmark(ctx)
	if err != nil {
		t.Fatalf("QuickBenchmark failed: %v", err)
	}
	if result == nil {
		t.Fatal("QuickBenchmark returned nil result")
	}
	if result.Score < 0 || result.Score > 100 {
		t.Errorf("Score out of range: %f", result.Score)
	}
}

func TestComputeQuickScore(t *testing.T) {
	tests := []struct {
		ping, jitter, speed float64
		name               string
	}{
		{10, 2, 100, "Excellent"},
		{30, 10, 50, "Good"},
		{100, 25, 20, "Fair"},
		{200, 50, 5, "Poor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ComputeQuickScore(tt.ping, tt.jitter, tt.speed)
			if score < 0 || score > 100 {
				t.Errorf("Score out of range [0,100]: %f", score)
			}
		})
	}
}

func TestFormatResults(t *testing.T) {
	r := &Result{
		DNSBenchmark: DNSSummary{
			FastestServer:    "1.1.1.1:53",
			FastestLatencyMs: 1.5,
			ImprovementMs:    5.2,
			AvgLatencyMs:     3.1,
		},
		LatencyBenchmark: LatencySummary{
			AvgLatencyMs: 25.3,
			MinLatencyMs: 18.1,
			MaxLatencyMs: 35.7,
			JitterMs:     4.2,
			PacketLoss:   0.0,
			QualityScore: 92.5,
		},
		OptimizationScore: 85.3,
		Timestamp:         time.Now().UnixMilli(),
	}

	s := FormatResults(r)
	if len(s) == 0 {
		t.Error("FormatResults returned empty string")
	}

	// Should contain key metrics
	if !contains(s, "85.3") {
		t.Error("Output should contain optimization score")
	}
	if !contains(s, "1.1.1.1") {
		t.Error("Output should contain fastest DNS server")
	}
	if !contains(s, "92.5") {
		t.Error("Output should contain quality score")
	}
}

func TestCompareOptimization(t *testing.T) {
	tests := []struct {
		before, after float64
		want          string
	}{
		{50, 75, "+50.0%"},
		{100, 50, "-50.0%"},
		{0, 50, "N/A"},
	}

	for _, tt := range tests {
		got := CompareOptimization(tt.before, tt.after)
		if got != tt.want {
			t.Errorf("CompareOptimization(%f, %f) = %s, want %s",
				tt.before, tt.after, got, tt.want)
		}
	}
}

func TestOptimizationScoreStability(t *testing.T) {
	score1 := computeOptimizationScore(&Result{
		DNSBenchmark:    DNSSummary{ImprovementMs: 10},
		LatencyBenchmark: LatencySummary{QualityScore: 80, PacketLoss: 0},
	})
	
	score2 := computeOptimizationScore(&Result{
		DNSBenchmark:    DNSSummary{ImprovementMs: 10},
		LatencyBenchmark: LatencySummary{QualityScore: 80, PacketLoss: 0},
	})
	
	if math.Abs(score1-score2) > 0.01 {
		t.Errorf("Scores should be stable: %f vs %f", score1, score2)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
