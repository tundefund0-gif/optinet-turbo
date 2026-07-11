package monitor

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestNewMonitor(t *testing.T) {
	m := NewMonitor(100)
	if m == nil {
		t.Fatal("NewMonitor returned nil")
	}
	if m.maxHistory != 100 {
		t.Errorf("Expected maxHistory 100, got %d", m.maxHistory)
	}
}

func TestMonitorUpdate(t *testing.T) {
	m := NewMonitor(10)
	
	m.Update(50.0, 5.0, 0.0, 15.0)
	metrics := m.GetMetrics()
	
	if metrics.Latency != 50.0 {
		t.Errorf("Expected latency 50.0, got %f", metrics.Latency)
	}
	if metrics.Jitter != 5.0 {
		t.Errorf("Expected jitter 5.0, got %f", metrics.Jitter)
	}
	if metrics.DNSSpeed != 15.0 {
		t.Errorf("Expected DNS speed 15.0, got %f", metrics.DNSSpeed)
	}
}

func TestMonitorHistory(t *testing.T) {
	m := NewMonitor(5)
	
	// Add 10 updates, should only keep last 5
	for i := 0; i < 10; i++ {
		m.Update(float64(i)*10, 0, 0, 0)
	}
	
	metrics := m.GetMetrics()
	if len(metrics.History) > 5 {
		t.Errorf("Expected at most 5 history points, got %d", len(metrics.History))
	}
}

func TestMonitorMinMax(t *testing.T) {
	m := NewMonitor(10)
	
	m.Update(100, 0, 0, 0)
	m.Update(50, 0, 0, 0)
	m.Update(200, 0, 0, 0)
	
	metrics := m.GetMetrics()
	if metrics.LatencyMin > metrics.LatencyMax {
		t.Error("Min should be <= Max")
	}
}

func TestMonitorSubscribe(t *testing.T) {
	m := NewMonitor(10)
	ch := m.Subscribe()
	
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
	
	m.Update(25.0, 2.0, 0.0, 10.0)
	
	select {
	case metrics := <-ch:
		if metrics.Latency != 25.0 {
			t.Errorf("Expected latency 25.0, got %f", metrics.Latency)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for subscribed metric")
	}
}

func TestCalculateJitter(t *testing.T) {
	tests := []struct {
		samples []float64
		want    float64
	}{
		{[]float64{10, 20, 30}, 10},
		{[]float64{10, 10, 10}, 0},
		{[]float64{50, 100, 50}, 50},
		{[]float64{}, 0},
		{[]float64{10}, 0},
	}
	
	for _, tt := range tests {
		got := CalculateJitter(tt.samples)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("CalculateJitter(%v) = %f, want %f", tt.samples, got, tt.want)
		}
	}
}

func TestComputeScore(t *testing.T) {
	tests := []struct {
		latency, jitter, loss, up, down float64
		name                            string
	}{
		{20, 5, 0, 10, 50, "Excellent connection"},
		{50, 15, 1, 5, 20, "Good connection"},
		{150, 40, 5, 2, 5, "Poor connection"},
		{300, 60, 15, 1, 1, "Bad connection"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ComputeScore(tt.latency, tt.jitter, tt.loss, tt.up, tt.down)
			if score < 0 || score > 100 {
				t.Errorf("Score out of range [0,100]: %f", score)
			}
		})
	}
	
	// Verify excellent > good > poor > bad
	excellent := ComputeScore(20, 5, 0, 10, 50)
	good := ComputeScore(50, 15, 1, 5, 20)
	poor := ComputeScore(150, 40, 5, 2, 5)
	
	if excellent <= good {
		t.Error("Expected excellent > good")
	}
	if good <= poor {
		t.Error("Expected good > poor")
	}
}

func TestGetNetworkScoreLabel(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{95, "Excellent"},
		{75, "Good"},
		{60, "Fair"},
		{40, "Poor"},
		{15, "Bad"},
	}
	
	for _, tt := range tests {
		got := GetNetworkScoreLabel(tt.score)
		if got != tt.want {
			t.Errorf("GetNetworkScoreLabel(%f) = %s, want %s", tt.score, got, tt.want)
		}
	}
}

func TestUpdateSpeeds(t *testing.T) {
	m := NewMonitor(10)
	m.UpdateSpeeds(100.0, 20.0)
	
	metrics := m.GetMetrics()
	if metrics.DownSpeed != 100.0 {
		t.Errorf("Expected down speed 100.0, got %f", metrics.DownSpeed)
	}
	if metrics.UpSpeed != 20.0 {
		t.Errorf("Expected up speed 20.0, got %f", metrics.UpSpeed)
	}
}

func TestSetConnections(t *testing.T) {
	m := NewMonitor(10)
	m.SetConnections(5)
	
	if metrics := m.GetMetrics(); metrics.Connections != 5 {
		t.Errorf("Expected 5 connections, got %d", metrics.Connections)
	}
}

func TestTestLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	latency, err := TestLatency(ctx, "8.8.8.8:443")
	if err != nil {
		t.Fatalf("TestLatency failed: %v", err)
	}
	if latency <= 0 {
		t.Error("Expected positive latency")
	}
}

func TestConcurrentUpdates(t *testing.T) {
	m := NewMonitor(100)
	
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			m.Update(float64(i), float64(i%10), 0, 0)
		}
		done <- true
	}()
	
	go func() {
		for i := 0; i < 100; i++ {
			m.GetMetrics()
		}
		done <- true
	}()
	
	<-done
	<-done
}

func BenchmarkMonitorUpdate(b *testing.B) {
	m := NewMonitor(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(50, 5, 0, 15)
	}
}

func BenchmarkMonitorGetMetrics(b *testing.B) {
	m := NewMonitor(100)
	for i := 0; i < 100; i++ {
		m.Update(50, 5, 0, 15)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.GetMetrics()
	}
}
