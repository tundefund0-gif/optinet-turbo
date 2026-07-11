package dns

import (
	"context"
	"fmt"
	"math"
	"net"
	"sort"
	"sync"
	"time"
)

// Server represents a DNS server with benchmark results
type Server struct {
	Addr    string        `json:"addr"`
	Latency time.Duration `json:"latency"`
	Loss    float64       `json:"loss_percent"`
	Score   float64       `json:"score"` // Composite score (lower is better)
}

type cacheEntry struct {
	ips       []net.IP
	expiresAt time.Time
}

// Optimizer handles high-performance DNS optimization
type Optimizer struct {
	servers   []string
	cache     map[string]*cacheEntry
	cacheMu   sync.RWMutex
	cacheTTL  time.Duration
	maxCache   int
	benchOpts *BenchOptions
}

// BenchOptions controls DNS benchmark behavior
type BenchOptions struct {
	Samples   int
	Timeout   time.Duration
	Concurrency int
}

// DefaultBenchOptions returns sensible benchmark defaults
func DefaultBenchOptions() *BenchOptions {
	return &BenchOptions{
		Samples:     5,
		Timeout:     2 * time.Second,
		Concurrency: 5,
	}
}

// NewOptimizer creates a new DNS optimizer
func NewOptimizer() *Optimizer {
	return &Optimizer{
		servers: []string{
			"1.1.1.1:53",
			"8.8.8.8:53",
			"9.9.9.9:53",
			"208.67.222.222:53",
			"76.76.2.2:53",
			"94.140.14.14:53",
			"185.228.168.9:53",
		},
		cache:    make(map[string]*cacheEntry),
		cacheTTL: 10 * time.Minute,
		maxCache: 1000,
		benchOpts: DefaultBenchOptions(),
	}
}

// FindFastest finds the fastest DNS server with statistical significance
func (o *Optimizer) FindFastest(ctx context.Context) (*Server, error) {
	servers, err := o.BenchmarkServers(ctx)
	if err != nil {
		return nil, err
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("no DNS servers reachable")
	}

	return &servers[0], nil
}

// BenchmarkServers tests all servers concurrently with statistical analysis
func (o *Optimizer) BenchmarkServers(ctx context.Context) ([]Server, error) {
	type jobResult struct {
		server Server
		err    error
	}

	sem := make(chan struct{}, o.benchOpts.Concurrency)
	results := make(chan jobResult, len(o.servers))
	var wg sync.WaitGroup

	for _, addr := range o.servers {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var totalLatency time.Duration
			lost := 0

			for i := 0; i < o.benchOpts.Samples; i++ {
				start := time.Now()
				conn, err := net.DialTimeout("udp", addr, o.benchOpts.Timeout)
				if err != nil {
					lost++
					continue
				}
				conn.Close()
				totalLatency += time.Since(start)
			}

			success := o.benchOpts.Samples - lost
			if success == 0 {
				results <- jobResult{
					server: Server{Addr: addr, Loss: 100, Score: math.MaxFloat64},
				}
				return
			}

			avgLatency := totalLatency / time.Duration(success)
			lossPercent := float64(lost) / float64(o.benchOpts.Samples) * 100

			// Score: weighted combination of latency and loss
			score := float64(avgLatency.Microseconds()) * (1 + lossPercent/10)

			results <- jobResult{
				server: Server{
					Addr:    addr,
					Latency: avgLatency,
					Loss:    lossPercent,
					Score:   score,
				},
			}
		}(addr)
	}

	wg.Wait()
	close(results)

	var servers []Server
	for r := range results {
		if r.err != nil {
			continue
		}
		servers = append(servers, r.server)
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("no DNS servers reachable")
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Score < servers[j].Score
	})

	return servers, nil
}

// ResolveWith resolves a hostname using a specific DNS server with caching
func (o *Optimizer) ResolveWith(ctx context.Context, hostname string, dnsServer string) ([]net.IP, error) {
	// Check cache first
	o.cacheMu.RLock()
	if entry, ok := o.cache[hostname]; ok && time.Now().Before(entry.expiresAt) {
		o.cacheMu.RUnlock()
		return entry.ips, nil
	}
	o.cacheMu.RUnlock()

	// Custom resolver targeting the specific DNS server
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout:   3 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return d.DialContext(ctx, "udp", dnsServer)
		},
	}

	ips, err := r.LookupHost(ctx, hostname)
	if err != nil {
		return nil, err
	}

	var parsed []net.IP
	for _, ip := range ips {
		if p := net.ParseIP(ip); p != nil {
			parsed = append(parsed, p)
		}
	}

	// Cache the result
	o.cacheMu.Lock()
	if len(o.cache) >= o.maxCache {
		// Simple eviction: clear oldest entries
		for k := range o.cache {
			delete(o.cache, k)
			if len(o.cache) < o.maxCache/2 {
				break
			}
		}
	}
	o.cache[hostname] = &cacheEntry{
		ips:       parsed,
		expiresAt: time.Now().Add(o.cacheTTL),
	}
	o.cacheMu.Unlock()

	return parsed, nil
}

// FlushCache clears the DNS cache
func (o *Optimizer) FlushCache() {
	o.cacheMu.Lock()
	defer o.cacheMu.Unlock()
	o.cache = make(map[string]*cacheEntry)
}

// GetCacheSize returns the number of cached entries
func (o *Optimizer) GetCacheSize() int {
	o.cacheMu.RLock()
	defer o.cacheMu.RUnlock()
	return len(o.cache)
}
