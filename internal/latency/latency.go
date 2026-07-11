package latency

import (
	"context"
	"fmt"
	"math"
	"net"
	"sort"
	"sync"
	"time"
)

// Sample represents a single latency measurement
type Sample struct {
	Latency   time.Duration `json:"latency"`
	Success   bool          `json:"success"`
	Timestamp int64         `json:"ts"`
	Size      int           `json:"size"`
}

// Stats holds statistical analysis of latency measurements
type Stats struct {
	Min      time.Duration `json:"min"`
	Max      time.Duration `json:"max"`
	Avg      time.Duration `json:"avg"`
	Median   time.Duration `json:"median"`
	P95      time.Duration `json:"p95"`
	P99      time.Duration `json:"p99"`
	StdDev   float64       `json:"stddev"`
	Jitter   time.Duration `json:"jitter"`
	Loss     float64       `json:"loss_percent"`
	Samples  int           `json:"samples"`
	Lost     int           `json:"lost"`
}

// Result holds latency test results for a target
type Result struct {
	Target    string        `json:"target"`
	Stats     Stats         `json:"stats"`
	Reachable bool          `json:"reachable"`
}

// Tester performs high-precision latency testing
type Tester struct {
	timeout    time.Duration
	interval   time.Duration
	sampleSize int
}

// NewTester creates a new latency tester with optimal defaults
func NewTester() *Tester {
	return &Tester{
		timeout:    2 * time.Second,
		interval:   100 * time.Millisecond,
		sampleSize: 5,
	}
}

// PingTarget measures latency to a target host:port
func (t *Tester) PingTarget(ctx context.Context, target string) (*Sample, error) {
	start := time.Now()
	d := net.Dialer{Timeout: t.timeout}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return &Sample{Success: false, Timestamp: time.Now().UnixMilli()}, nil
	}
	latency := time.Since(start)
	conn.Close()

	return &Sample{
		Latency:   latency,
		Success:   true,
		Timestamp: start.UnixMilli(),
	}, nil
}

// BenchmarkTarget performs multiple pings with statistical analysis
func (t *Tester) BenchmarkTarget(ctx context.Context, target string, samples int) (*Stats, error) {
	if samples <= 0 {
		samples = t.sampleSize
	}

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		results  []float64
		success  int
		lost     int
	)

	// Use a ticker to space out pings
	for i := 0; i < samples; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			start := time.Now()
			d := net.Dialer{Timeout: t.timeout}
			conn, err := d.DialContext(ctx, "tcp", target)
			if err != nil {
				mu.Lock()
				lost++
				mu.Unlock()
				return
			}
			latency := time.Since(start)
			conn.Close()

			mu.Lock()
			results = append(results, float64(latency.Microseconds()))
			success++
			mu.Unlock()
		}()

		// Small delay between pings to avoid overwhelming
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil, ctx.Err()
		case <-time.After(t.interval):
		}
	}

	wg.Wait()

	if success == 0 {
		return &Stats{
			Loss:    100,
			Samples: samples,
			Lost:    samples,
		}, nil
	}

	return computeStats(results, success, lost, samples), nil
}

// BenchmarkTargets concurrently benchmarks multiple targets
func (t *Tester) BenchmarkTargets(ctx context.Context, targets []string) ([]Result, error) {
	type jobResult struct {
		result Result
		err    error
	}

	results := make(chan jobResult, len(targets))
	var wg sync.WaitGroup

	for _, target := range targets {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			stats, err := t.BenchmarkTarget(ctx, target, t.sampleSize)
			if err != nil || stats == nil {
				results <- jobResult{
					result: Result{
						Target: target,
						Stats:  Stats{Loss: 100, Samples: t.sampleSize, Lost: t.sampleSize},
					},
					err: err,
				}
				return
			}
			results <- jobResult{
				result: Result{
					Target:    target,
					Stats:     *stats,
					Reachable: stats.Lost < stats.Samples,
				},
			}
		}(target)
	}

	wg.Wait()
	close(results)

	var resultList []Result
	for r := range results {
		if r.err != nil {
			continue
		}
		resultList = append(resultList, r.result)
	}

	sort.Slice(resultList, func(i, j int) bool {
		return resultList[i].Stats.Avg < resultList[j].Stats.Avg
	})

	return resultList, nil
}

func computeStats(results []float64, success, lost, totalSamples int) *Stats {
	sort.Float64s(results)

	var sum float64
	for _, v := range results {
		sum += v
	}
	avg := sum / float64(success)

	// Standard deviation
	var varianceSum float64
	for _, v := range results {
		diff := v - avg
		varianceSum += diff * diff
	}
	stdDev := math.Sqrt(varianceSum / float64(success))

	// Percentiles
	median := percentile(results, 50)
	p95 := percentile(results, 95)
	p99 := percentile(results, 99)

	// Jitter (mean absolute difference between consecutive samples)
	var jitterSum float64
	for i := 1; i < len(results); i++ {
		jitterSum += math.Abs(results[i] - results[i-1])
	}
	jitter := time.Duration(jitterSum/float64(len(results)-1)) * time.Microsecond

	lossPercent := float64(lost) / float64(totalSamples) * 100

	return &Stats{
		Min:     time.Duration(results[0]) * time.Microsecond,
		Max:     time.Duration(results[len(results)-1]) * time.Microsecond,
		Avg:     time.Duration(avg) * time.Microsecond,
		Median:  time.Duration(median) * time.Microsecond,
		P95:     time.Duration(p95) * time.Microsecond,
		P99:     time.Duration(p99) * time.Microsecond,
		StdDev:  stdDev,
		Jitter:  jitter,
		Loss:    math.Round(lossPercent*100) / 100,
		Samples: totalSamples,
		Lost:    lost,
	}
}

func percentile(data []float64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}
	if p <= 0 {
		return data[0]
	}
	if p >= 100 {
		return data[len(data)-1]
	}

	rank := p / 100 * float64(len(data)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))

	if lower == upper {
		return data[lower]
	}

	return data[lower] + (data[upper]-data[lower])*(rank-float64(lower))
}

// GameTargets returns common game/CDN server targets
func GameTargets() []string {
	return []string{
		"8.8.8.8:443",
		"1.1.1.1:443",
		"9.9.9.9:443",
		"208.67.222.222:443",
	}
}

// FindFastestTarget benchmarks all targets and returns the fastest
func (t *Tester) FindFastestTarget(ctx context.Context, targets []string) (*Result, error) {
	results, err := t.BenchmarkTargets(ctx, targets)
	if err != nil {
		return nil, err
	}

	for _, r := range results {
		if r.Reachable {
			return &r, nil
		}
	}

	return nil, fmt.Errorf("no reachable targets found")
}
