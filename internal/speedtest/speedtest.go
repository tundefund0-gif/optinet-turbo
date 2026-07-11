package speedtest

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Result holds complete speed test results
type Result struct {
	DownloadMbps  float64        `json:"download_mbps"`
	UploadMbps    float64        `json:"upload_mbps"`
	LatencyMs     float64        `json:"latency_ms"`
	JitterMs      float64        `json:"jitter_ms"`
	PacketLoss    float64        `json:"packet_loss"`
	Server        string         `json:"server"`
	Timestamp     int64          `json:"timestamp"`
	Details       *TestDetails   `json:"details,omitempty"`
}

// TestDetails provides detailed breakdown
type TestDetails struct {
	DownloadSamples []float64 `json:"download_samples"`
	LatencySamples  []float64 `json:"latency_samples"`
	ServerIP        string    `json:"server_ip"`
	ServerLocation  string    `json:"server_location"`
}

// Tester performs comprehensive speed tests
type Tester struct {
	client     *http.Client
	downloadURLs []string
	sampleSize   int
	mu           sync.Mutex
	cachedResult *Result
	cachedAt     time.Time
	cacheTTL     time.Duration
}

// NewTester creates a new speed test engine
func NewTester() *Tester {
	tr := &http.Transport{
		MaxIdleConns:        20,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   false,
	}

	return &Tester{
		client: &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
		},
		downloadURLs: []string{
			"http://speedtest.tele2.net/5MB.zip",
			"http://speedtest.tele2.net/1MB.zip",
			"https://proof.ovh.net/files/10Mb.dat",
		},
		sampleSize: 3,
		cacheTTL:   5 * time.Minute,
	}
}

// RunFullTest performs a complete benchmark of the network
func (t *Tester) RunFullTest(ctx context.Context) (*Result, error) {
	t.mu.Lock()
	if t.cachedResult != nil && time.Since(t.cachedAt) < t.cacheTTL {
		result := *t.cachedResult
		t.mu.Unlock()
		return &result, nil
	}
	t.mu.Unlock()

	result := &Result{
		Timestamp: time.Now().UnixMilli(),
	}

	// 1. Latency test
	latencySamples, err := t.measureLatency(ctx, 10)
	if err == nil && len(latencySamples) > 0 {
		result.LatencyMs = avg(latencySamples)
		result.JitterMs = computeJitter(latencySamples)
		
		// Count loss
		lost := 0
		for _, s := range latencySamples {
			if s <= 0 {
				lost++
			}
		}
		result.PacketLoss = float64(lost) / float64(len(latencySamples)) * 100
	}

	// 2. Download speed test
	downloadSamples, err := t.measureDownload(ctx)
	if err == nil && len(downloadSamples) > 0 {
		result.DownloadMbps = maxValue(downloadSamples)
		if result.Details == nil {
			result.Details = &TestDetails{}
		}
		result.Details.DownloadSamples = downloadSamples
	}

	// Cache the result
	t.mu.Lock()
	t.cachedResult = result
	t.cachedAt = time.Now()
	t.mu.Unlock()

	return result, nil
}

func (t *Tester) measureLatency(ctx context.Context, samples int) ([]float64, error) {
	var results []float64
	var mu sync.Mutex
	var wg sync.WaitGroup

	targets := []string{
		"8.8.8.8:443",
		"1.1.1.1:443",
		"9.9.9.9:443",
	}

	for _, target := range targets {
		for i := 0; i < samples/len(targets); i++ {
			wg.Add(1)
			go func(addr string) {
				defer wg.Done()
				start := time.Now()
				d := net.Dialer{Timeout: 2 * time.Second}
				conn, err := d.DialContext(ctx, "tcp", addr)
				if err != nil {
					mu.Lock()
					results = append(results, 0)
					mu.Unlock()
					return
				}
				latency := float64(time.Since(start).Microseconds()) / 1000.0
				conn.Close()
				mu.Lock()
				results = append(results, latency)
				mu.Unlock()
			}(target)
			time.Sleep(50 * time.Millisecond)
		}
	}

	wg.Wait()
	
	if len(results) == 0 {
		return nil, fmt.Errorf("no latency samples")
	}

	return results, nil
}

func (t *Tester) measureDownload(ctx context.Context) ([]float64, error) {
	var samples []float64
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, url := range t.downloadURLs {
		wg.Add(1)
		go func(targetURL string) {
			defer wg.Done()

			req, _ := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
			resp, err := t.client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			var totalBytes int64
			buf := make([]byte, 64*1024)
			readStart := time.Now()
			timeout := time.After(5 * time.Second)

			for {
				select {
				case <-ctx.Done():
					return
				case <-timeout:
					mu.Lock()
					elapsed := time.Since(readStart).Seconds()
					if elapsed > 0 {
						speed := (float64(totalBytes) * 8) / (elapsed * 1000 * 1000)
						samples = append(samples, math.Round(speed*100)/100)
					}
					mu.Unlock()
					return
				default:
				}

				n, err := resp.Body.Read(buf)
				totalBytes += int64(n)
				if err == io.EOF {
					break
				}
			}

			elapsed := time.Since(readStart).Seconds()
			if elapsed > 0 {
				speed := (float64(totalBytes) * 8) / (elapsed * 1000 * 1000)
				mu.Lock()
				samples = append(samples, math.Round(speed*100)/100)
				mu.Unlock()
			}


		}(url)
	}

	wg.Wait()

	if len(samples) == 0 {
		return nil, fmt.Errorf("no download samples")
	}

	return samples, nil
}

// FastPing does a quick single-target latency check
func FastPing(ctx context.Context, target string) (float64, error) {
	start := time.Now()
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return 0, err
	}
	conn.Close()
	return float64(time.Since(start).Microseconds()) / 1000.0, nil
}

// CompareResult stores before/after comparison
type CompareResult struct {
	Before *Result `json:"before"`
	After  *Result `json:"after"`
	Improvement struct {
		LatencyPercent  float64 `json:"latency_percent"`
		SpeedPercent    float64 `json:"speed_percent"`
		JitterPercent   float64 `json:"jitter_percent"`
	} `json:"improvement"`
}

// Compare compares two speed test results
func Compare(before, after *Result) *CompareResult {
	r := &CompareResult{Before: before, After: after}

	if before.LatencyMs > 0 {
		r.Improvement.LatencyPercent = ((before.LatencyMs - after.LatencyMs) / before.LatencyMs) * 100
	}
	if before.DownloadMbps > 0 {
		r.Improvement.SpeedPercent = ((after.DownloadMbps - before.DownloadMbps) / before.DownloadMbps) * 100
	}
	if before.JitterMs > 0 {
		r.Improvement.JitterPercent = ((before.JitterMs - after.JitterMs) / before.JitterMs) * 100
	}

	return r
}

func avg(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	var sum float64
	count := 0
	for _, v := range data {
		if v > 0 {
			sum += v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return math.Round(sum/float64(count)*100) / 100
}

func computeJitter(data []float64) float64 {
	var valid []float64
	for _, v := range data {
		if v > 0 {
			valid = append(valid, v)
		}
	}
	if len(valid) < 2 {
		return 0
	}
	var sum float64
	for i := 1; i < len(valid); i++ {
		sum += math.Abs(valid[i] - valid[i-1])
	}
	return math.Round(sum/float64(len(valid)-1)*100) / 100
}

func maxValue(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	max := data[0]
	for _, v := range data {
		if v > max {
			max = v
		}
	}
	return max
}

// SortResults sorts speed test results by download speed (descending)
func SortResults(results []Result) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].DownloadMbps > results[j].DownloadMbps
	})
}
