package monitor

import (
	"context"
	"io"
	"math"
	"net"
	"net/http"
	"sync"
	"time"
)

// Point represents a single data point for charting
type Point struct {
	Timestamp int64   `json:"ts"`
	Value     float64 `json:"value"`
	Label     string  `json:"label,omitempty"`
}

// NetworkMetrics holds complete network performance data
type NetworkMetrics struct {
	Latency     float64   `json:"latency"`
	LatencyMax  float64   `json:"latency_max"`
	LatencyMin  float64   `json:"latency_min"`
	Jitter      float64   `json:"jitter"`
	PacketLoss  float64   `json:"packet_loss"`
	DNSSpeed    float64   `json:"dns_speed"`
	UpSpeed     float64   `json:"up_speed"`
	DownSpeed   float64   `json:"down_speed"`
	Connections int32     `json:"connections"`
	ThroughputIn  float64 `json:"throughput_in"`
	ThroughputOut float64 `json:"throughput_out"`
	Score       float64   `json:"score"`
	Timestamp   int64     `json:"timestamp"`
	History     []Point   `json:"history,omitempty"`
	JitterHistory []Point `json:"jitter_history,omitempty"`
}

// ComputeScore calculates a network quality score (0-100, higher = better)
func ComputeScore(latency, jitter, loss, upSpeed, downSpeed float64) float64 {
	// Lower latency is better (penalize over 150ms)
	latencyScore := math.Max(0, 100-latency/3)
	if latency > 150 {
		latencyScore = math.Max(0, 50-(latency-150)/10)
	}
	
	// Jitter should be under 30ms
	jitterScore := math.Max(0, 100-jitter*3)
	
	// 0% loss = 100, 10%+ loss = 0
	lossScore := math.Max(0, 100-loss*10)
	
	// Speed score (30Mbps+ down = 100)
	downScore := math.Min(100, downSpeed/0.3)
	upScore := math.Min(100, upSpeed/0.1)
	
	// Weighted composite (gaming-focused: latency and loss matter most)
	score := (latencyScore*0.35 + jitterScore*0.15 + lossScore*0.25 + downScore*0.15 + upScore*0.10)
	return math.Round(score*100) / 100
}

// Monitor tracks network performance with real measurements
type Monitor struct {
	mu              sync.RWMutex
	metrics         NetworkMetrics
	latencyHistory  []float64
	jitterHistory   []float64
	maxHistory      int
	history         []Point
	jitterPoints    []Point
	subscribers     []chan NetworkMetrics
	client          *http.Client
}

// NewMonitor creates a new network monitor
func NewMonitor(maxHistory int) *Monitor {
	if maxHistory <= 0 {
		maxHistory = 120
	}
	
	tr := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   false,
	}
	
	return &Monitor{
		maxHistory: maxHistory,
		client: &http.Client{
			Transport: tr,
			Timeout:   10 * time.Second,
		},
		metrics: NetworkMetrics{
			Timestamp: time.Now().UnixMilli(),
		},
	}
}

// Update updates the monitor with new metrics
func (m *Monitor) Update(latency, jitter, packetLoss, dnsSpeed float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics.Latency = latency
	m.metrics.Jitter = jitter
	m.metrics.PacketLoss = packetLoss
	m.metrics.DNSSpeed = dnsSpeed
	m.metrics.Timestamp = time.Now().UnixMilli()

	// Update latency history
	point := Point{
		Timestamp: m.metrics.Timestamp,
		Value:     latency,
		Label:     "ping",
	}
	m.history = append(m.history, point)
	if len(m.history) > m.maxHistory {
		m.history = m.history[len(m.history)-m.maxHistory:]
	}

	// Update jitter history
	jitterPoint := Point{
		Timestamp: m.metrics.Timestamp,
		Value:     jitter,
		Label:     "jitter",
	}
	m.jitterPoints = append(m.jitterPoints, jitterPoint)
	if len(m.jitterPoints) > m.maxHistory {
		m.jitterPoints = m.jitterPoints[len(m.jitterPoints)-m.maxHistory:]
	}

	m.latencyHistory = append(m.latencyHistory, latency)
	if len(m.latencyHistory) > m.maxHistory {
		m.latencyHistory = m.latencyHistory[len(m.latencyHistory)-m.maxHistory:]
	}

	m.jitterHistory = append(m.jitterHistory, jitter)
	if len(m.jitterHistory) > m.maxHistory {
		m.jitterHistory = m.jitterHistory[len(m.jitterHistory)-m.maxHistory:]
	}

	// Calculate min/max
	if len(m.latencyHistory) > 0 {
		min := m.latencyHistory[0]
		max := m.latencyHistory[0]
		for _, v := range m.latencyHistory {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		m.metrics.LatencyMin = min
		m.metrics.LatencyMax = max
	}

	// Update score
	m.metrics.Score = ComputeScore(
		latency, jitter, packetLoss,
		m.metrics.UpSpeed, m.metrics.DownSpeed,
	)

	// Notify subscribers
	metrics := m.metrics
	metrics.History = m.history
	metrics.JitterHistory = m.jitterPoints

	for _, ch := range m.subscribers {
		select {
		case ch <- metrics:
		default:
		}
	}
}

// UpdateSpeeds updates bandwidth measurements
func (m *Monitor) UpdateSpeeds(down, up float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics.DownSpeed = down
	m.metrics.UpSpeed = up
}

// UpdateThroughput updates real-time throughput
func (m *Monitor) UpdateThroughput(in, out float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics.ThroughputIn = in
	m.metrics.ThroughputOut = out
}

// GetMetrics returns current metrics
func (m *Monitor) GetMetrics() NetworkMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	metrics := m.metrics
	metrics.History = m.history
	metrics.JitterHistory = m.jitterPoints
	return metrics
}

// Subscribe returns a channel that receives metric updates
func (m *Monitor) Subscribe() <-chan NetworkMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan NetworkMetrics, 16)
	m.subscribers = append(m.subscribers, ch)
	return ch
}

// SetConnections updates the active connection count
func (m *Monitor) SetConnections(count int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics.Connections = count
}

// CalculateJitter computes jitter from latency samples
func CalculateJitter(samples []float64) float64 {
	if len(samples) < 2 {
		return 0
	}

	var sum float64
	for i := 1; i < len(samples); i++ {
		diff := math.Abs(samples[i] - samples[i-1])
		sum += diff
	}
	return math.Round(sum/float64(len(samples)-1)*100) / 100
}

// TestLatency measures latency to a target in milliseconds
func TestLatency(ctx context.Context, target string) (float64, error) {
	start := time.Now()
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return 0, err
	}
	conn.Close()
	return float64(time.Since(start).Microseconds()) / 1000.0, nil
}

// SpeedTestResult holds speed test measurements
type SpeedTestResult struct {
	DownloadMbps float64 `json:"download_mbps"`
	UploadMbps   float64 `json:"upload_mbps"`
	LatencyMs    float64 `json:"latency_ms"`
	Server       string  `json:"server"`
	Duration     float64 `json:"duration_sec"`
}

// RunSpeedTest performs a real download speed test against a target
func RunSpeedTest(ctx context.Context, targetURL string, duration time.Duration) (*SpeedTestResult, error) {
	client := &http.Client{
		Timeout: duration + 5*time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	start := time.Now()
	
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Measure download speed
	var totalBytes int64
	buf := make([]byte, 64*1024)
	readStart := time.Now()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		totalBytes += int64(n)
		if err == io.EOF || time.Since(readStart) >= duration {
			break
		}
		if err != nil {
			break
		}
	}

	elapsed := time.Since(readStart).Seconds()
	if elapsed <= 0 {
		elapsed = 0.001
	}

	downloadMbps := (float64(totalBytes) * 8) / (elapsed * 1000 * 1000)

	return &SpeedTestResult{
		DownloadMbps: math.Round(downloadMbps*100) / 100,
		LatencyMs:    float64(time.Since(start).Microseconds()) / 1000.0,
		Duration:     elapsed,
		Server:       targetURL,
	}, nil
}

// DefaultSpeedTestTargets returns URLs for speed testing
func DefaultSpeedTestTargets() []string {
	return []string{
		"http://speedtest.tele2.net/10MB.zip",
		"http://speedtest.tele2.net/5MB.zip",
		"http://speedtest.tele2.net/1MB.zip",
	}
}

// GetNetworkScoreLabel returns a human-readable label for a score
func GetNetworkScoreLabel(score float64) string {
	switch {
	case score >= 90:
		return "Excellent"
	case score >= 70:
		return "Good"
	case score >= 50:
		return "Fair"
	case score >= 30:
		return "Poor"
	default:
		return "Bad"
	}
}
