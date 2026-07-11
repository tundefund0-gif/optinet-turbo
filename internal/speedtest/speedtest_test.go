package speedtest

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
	if len(tester.downloadURLs) == 0 {
		t.Error("No download URLs configured")
	}
}

func TestAvg(t *testing.T) {
	tests := []struct {
		data []float64
		want float64
	}{
		{[]float64{10, 20, 30}, 20},
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{0, 10, 0}, 10},
	}
	
	for _, tt := range tests {
		got := avg(tt.data)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("avg(%v) = %f, want %f", tt.data, got, tt.want)
		}
	}
}

func TestComputeJitter(t *testing.T) {
	tests := []struct {
		data []float64
		want float64
	}{
		{[]float64{10, 20, 30}, 10},
		{[]float64{10, 10, 10}, 0},
		{[]float64{}, 0},
		{[]float64{10}, 0},
		{[]float64{0, 10, 0, 10}, 0}, // zeros get filtered, then 10,10 = 0 jitter
	}
	
	for _, tt := range tests {
		got := computeJitter(tt.data)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("computeJitter(%v) = %f, want %f", tt.data, got, tt.want)
		}
	}
}

func TestMaxValue(t *testing.T) {
	tests := []struct {
		data []float64
		want float64
	}{
		{[]float64{10, 20, 30}, 30},
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{-10, -5}, -5},
	}
	
	for _, tt := range tests {
		got := maxValue(tt.data)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("maxValue(%v) = %f, want %f", tt.data, got, tt.want)
		}
	}
}

func TestFastPing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	latency, err := FastPing(ctx, "8.8.8.8:443")
	if err != nil {
		t.Fatalf("FastPing failed: %v", err)
	}
	if latency <= 0 {
		t.Error("Expected positive latency")
	}
}

func TestCompare(t *testing.T) {
	before := &Result{LatencyMs: 100, DownloadMbps: 50, JitterMs: 20}
	after := &Result{LatencyMs: 50, DownloadMbps: 100, JitterMs: 10}
	
	cmp := Compare(before, after)
	if cmp.Improvement.LatencyPercent != 50.0 {
		t.Errorf("Expected 50%% latency improvement, got %.1f%%", cmp.Improvement.LatencyPercent)
	}
	if cmp.Improvement.SpeedPercent != 100.0 {
		t.Errorf("Expected 100%% speed improvement, got %.1f%%", cmp.Improvement.SpeedPercent)
	}
	if cmp.Improvement.JitterPercent != 50.0 {
		t.Errorf("Expected 50%% jitter improvement, got %.1f%%", cmp.Improvement.JitterPercent)
	}
	
	if cmp.Before != before {
		t.Error("Compare.Before should reference the original")
	}
	if cmp.After != after {
		t.Error("Compare.After should reference the original")
	}
}

func TestSortResults(t *testing.T) {
	results := []Result{
		{DownloadMbps: 10},
		{DownloadMbps: 100},
		{DownloadMbps: 50},
	}
	
	SortResults(results)
	
	if results[0].DownloadMbps != 100 {
		t.Error("Expected highest speed first after sort")
	}
	if results[2].DownloadMbps != 10 {
		t.Error("Expected lowest speed last after sort")
	}
}

func BenchmarkFastPing(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping network benchmark")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FastPing(ctx, "8.8.8.8:443")
	}
}
