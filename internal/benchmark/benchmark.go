package benchmark

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/user/optinet/internal/dns"
	"github.com/user/optinet/internal/latency"
	"github.com/user/optinet/internal/speedtest"
)

// Suite runs comprehensive network benchmarks
type Suite struct {
	dnsOpt     *dns.Optimizer
	latTester  *latency.Tester
	speedTester *speedtest.Tester
}

// Result holds all benchmark results
type Result struct {
	DNSBenchmark   DNSSummary      `json:"dns"`
	LatencyBenchmark LatencySummary `json:"latency"`
	SpeedTest      *speedtest.Result `json:"speed"`
	OptimizationScore float64       `json:"optimization_score"`
	Timestamp      int64           `json:"timestamp"`
}

// DNSSummary summarizes DNS benchmark results
type DNSSummary struct {
	FastestServer    string  `json:"fastest_server"`
	FastestLatencyMs float64 `json:"fastest_latency_ms"`
	Servers          []dns.Server `json:"servers"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
	ImprovementMs    float64 `json:"improvement_ms"`
}

// LatencySummary summarizes latency test results
type LatencySummary struct {
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	MinLatencyMs    float64 `json:"min_latency_ms"`
	MaxLatencyMs    float64 `json:"max_latency_ms"`
	JitterMs        float64 `json:"jitter_ms"`
	PacketLoss      float64 `json:"packet_loss"`
	Targets         []latency.Result `json:"targets"`
	QualityScore    float64 `json:"quality_score"`
}

// NewSuite creates a new benchmark suite
func NewSuite() *Suite {
	return &Suite{
		dnsOpt:      dns.NewOptimizer(),
		latTester:   latency.NewTester(),
		speedTester: speedtest.NewTester(),
	}
}

// RunAll runs all benchmarks and returns comprehensive results
func (b *Suite) RunAll(ctx context.Context) (*Result, error) {
	result := &Result{
		Timestamp: time.Now().UnixMilli(),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// DNS benchmark
	wg.Add(1)
	go func() {
		defer wg.Done()
		servers, err := b.dnsOpt.BenchmarkServers(ctx)
		if err != nil {
			return
		}

		summary := DNSSummary{Servers: servers}
		if len(servers) > 0 {
			summary.FastestServer = servers[0].Addr
			summary.FastestLatencyMs = float64(servers[0].Latency.Microseconds()) / 1000.0
			
			var total float64
			for _, s := range servers {
				total += float64(s.Latency.Microseconds()) / 1000.0
			}
			summary.AvgLatencyMs = math.Round(total/float64(len(servers))*100) / 100

			// Calculate improvement over worst
			if len(servers) > 1 {
				worstLatency := float64(servers[len(servers)-1].Latency.Microseconds()) / 1000.0
				summary.ImprovementMs = math.Round((worstLatency-summary.FastestLatencyMs)*100) / 100
			}
		}

		mu.Lock()
		result.DNSBenchmark = summary
		mu.Unlock()
	}()

	// Latency benchmark
	wg.Add(1)
	go func() {
		defer wg.Done()
		targets := []string{
			"8.8.8.8:443",
			"1.1.1.1:443",
			"9.9.9.9:443",
			"208.67.222.222:443",
		}

		results, err := b.latTester.BenchmarkTargets(ctx, targets)
		if err != nil {
			return
		}

		summary := LatencySummary{Targets: results}
		var totalLat, minLat, maxLat float64
		var count int

		for _, r := range results {
			if r.Reachable {
				lat := float64(r.Stats.Avg.Microseconds()) / 1000.0
				totalLat += lat
				count++
				if count == 1 || lat < minLat {
					minLat = lat
				}
				if lat > maxLat {
					maxLat = lat
				}
			}
		}

		if count > 0 {
			summary.AvgLatencyMs = math.Round(totalLat/float64(count)*100) / 100
			summary.MinLatencyMs = math.Round(minLat*100) / 100
			summary.MaxLatencyMs = math.Round(maxLat*100) / 100
		}

		// Jitter from results
		var jitterSum float64
		jitterCount := 0
		for i := 1; i < len(results); i++ {
			if results[i].Reachable && results[i-1].Reachable {
				l1 := float64(results[i].Stats.Avg.Microseconds()) / 1000.0
				l2 := float64(results[i-1].Stats.Avg.Microseconds()) / 1000.0
				jitterSum += math.Abs(l1 - l2)
				jitterCount++
			}
		}
		if jitterCount > 0 {
			summary.JitterMs = math.Round(jitterSum/float64(jitterCount)*100) / 100
		}

		// Packet loss
		var totalSamples, lost int
		for _, r := range results {
			totalSamples += r.Stats.Samples
			lost += r.Stats.Lost
		}
		if totalSamples > 0 {
			summary.PacketLoss = math.Round(float64(lost)/float64(totalSamples)*10000) / 100
		}

		// Quality score (based on latency)
		if summary.AvgLatencyMs > 0 {
			score := 100.0
			if summary.AvgLatencyMs > 20 {
				score -= (summary.AvgLatencyMs - 20) * 0.5
			}
			if summary.JitterMs > 10 {
				score -= (summary.JitterMs - 10) * 2
			}
			if summary.PacketLoss > 1 {
				score -= summary.PacketLoss * 5
			}
			summary.QualityScore = math.Max(0, math.Round(score*100)/100)
		}

		mu.Lock()
		result.LatencyBenchmark = summary
		mu.Unlock()
	}()

	wg.Wait()

	// Overall optimization score
	result.OptimizationScore = computeOptimizationScore(result)

	return result, nil
}

func computeOptimizationScore(r *Result) float64 {
	score := 50.0 // Start at 50

	// DNS improvement
	if r.DNSBenchmark.ImprovementMs > 0 {
		score += math.Min(20, r.DNSBenchmark.ImprovementMs*2)
	}

	// Latency quality
	if r.LatencyBenchmark.QualityScore > 0 {
		diff := r.LatencyBenchmark.QualityScore - 50
		score += diff * 0.3
	}

	// Packet loss
	if r.LatencyBenchmark.PacketLoss == 0 {
		score += 10
	} else if r.LatencyBenchmark.PacketLoss < 1 {
		score += 5
	}

	return math.Max(0, math.Min(100, math.Round(score*100)/100))
}

// FormatResults formats benchmark results as a readable string
func FormatResults(r *Result) string {
	var s string

	s += "╔════════════════════════════════════════╗\n"
	s += "║        OptiNet Benchmark Report        ║\n"
	s += "╚════════════════════════════════════════╝\n\n"

	s += fmt.Sprintf("Network Score: %.1f/100\n", r.OptimizationScore)
	s += fmt.Sprintf("Timestamp:     %s\n\n", time.UnixMilli(r.Timestamp).Format(time.RFC3339))

	s += "── DNS Servers ──\n"
	if r.DNSBenchmark.FastestServer != "" {
		s += fmt.Sprintf("  Fastest:     %s (%.2f ms)\n",
			r.DNSBenchmark.FastestServer, r.DNSBenchmark.FastestLatencyMs)
		s += fmt.Sprintf("  Improvement: %.2f ms\n", r.DNSBenchmark.ImprovementMs)
		s += fmt.Sprintf("  Average:     %.2f ms\n", r.DNSBenchmark.AvgLatencyMs)
	}
	s += "\n"

	s += "── Latency ──\n"
	s += fmt.Sprintf("  Average:     %.1f ms\n", r.LatencyBenchmark.AvgLatencyMs)
	s += fmt.Sprintf("  Minimum:     %.1f ms\n", r.LatencyBenchmark.MinLatencyMs)
	s += fmt.Sprintf("  Maximum:     %.1f ms\n", r.LatencyBenchmark.MaxLatencyMs)
	s += fmt.Sprintf("  Jitter:      %.1f ms\n", r.LatencyBenchmark.JitterMs)
	s += fmt.Sprintf("  Packet Loss: %.1f%%\n", r.LatencyBenchmark.PacketLoss)
	s += fmt.Sprintf("  Quality:     %.1f/100\n", r.LatencyBenchmark.QualityScore)
	s += "\n"

	s += "── Target Breakdown ──\n"
	for _, t := range r.LatencyBenchmark.Targets {
		if t.Reachable {
			s += fmt.Sprintf("  %-20s %6.1f ms  (loss: %.1f%%)\n",
				t.Target,
				float64(t.Stats.Avg.Microseconds())/1000.0,
				t.Stats.Loss)
		} else {
			s += fmt.Sprintf("  %-20s  OFFLINE\n", t.Target)
		}
	}

	return s
}

// CompareOptimization shows before/after improvement percentage
func CompareOptimization(before, after float64) string {
	if before == 0 {
		return "N/A"
	}
	improvement := ((after - before) / before) * 100
	sign := "+"
	if improvement < 0 {
		sign = ""
	}
	return fmt.Sprintf("%s%.1f%%", sign, improvement)
}

// QuickResult holds a simple benchmark result for fast display
type QuickResult struct {
	PingMs    float64 `json:"ping_ms"`
	JitterMs  float64 `json:"jitter_ms"`
	SpeedMbps float64 `json:"speed_mbps"`
	Score     float64 `json:"score"`
}

// QuickBenchmark runs a fast (5 second) benchmark
func QuickBenchmark(ctx context.Context) (*QuickResult, error) {
	latTester := latency.NewTester()
	speedTester := speedtest.NewTester()

	result := &QuickResult{}

	// Quick latency test
	results, err := latTester.BenchmarkTargets(ctx, []string{"8.8.8.8:443", "1.1.1.1:443"})
	if err == nil && len(results) > 0 {
		var total float64
		var count int
		var jitterSum float64
		var prev float64
		for _, r := range results {
			if r.Reachable {
				lat := float64(r.Stats.Avg.Microseconds()) / 1000.0
				total += lat
				count++
				if prev > 0 {
					jitterSum += math.Abs(lat - prev)
				}
				prev = lat
			}
		}
		if count > 0 {
			result.PingMs = math.Round(total/float64(count)*100) / 100
		}
		if count > 1 {
			result.JitterMs = math.Round(jitterSum/float64(count-1)*100) / 100
		}
	}

	// Speed test
	speedResult, err := speedTester.RunFullTest(ctx)
	if err == nil {
		result.SpeedMbps = speedResult.DownloadMbps
	}

	// Score
	result.Score = ComputeQuickScore(result.PingMs, result.JitterMs, result.SpeedMbps)

	return result, nil
}

// ComputeQuickScore calculates a quick network score (0-100)
func ComputeQuickScore(ping, jitter, speed float64) float64 {
	score := 60.0

	if ping > 0 {
		if ping < 20 {
			score += 30
		} else if ping < 50 {
			score += 20
		} else if ping < 100 {
			score += 10
		} else {
			score -= (ping - 100) * 0.2
		}
	}

	if jitter > 0 && jitter < 15 {
		score += 10
	} else if jitter > 30 {
		score -= 10
	}

	if speed > 50 {
		score += 10
	} else if speed > 20 {
		score += 5
	} else if speed < 5 {
		score -= 10
	}

	return math.Max(0, math.Min(100, math.Round(score*100)/100))
}
