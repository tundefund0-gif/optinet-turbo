package dns

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestNewOptimizer(t *testing.T) {
	o := NewOptimizer()
	if o == nil {
		t.Fatal("NewOptimizer returned nil")
	}
	if len(o.servers) == 0 {
		t.Error("No DNS servers configured")
	}
}

func TestCache(t *testing.T) {
	o := NewOptimizer()
	
	// Test cache miss
	ips := []net.IP{net.ParseIP("1.2.3.4")}
	o.cache["test.example.com"] = &cacheEntry{
		ips:       ips,
		expiresAt: time.Now().Add(time.Hour),
	}
	
	if o.GetCacheSize() != 1 {
		t.Errorf("expected 1 cached entry, got %d", o.GetCacheSize())
	}
	
	// Test flush
	o.FlushCache()
	if o.GetCacheSize() != 0 {
		t.Errorf("expected 0 cached entries after flush, got %d", o.GetCacheSize())
	}
}

func TestFindFastest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	
	o := NewOptimizer()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	server, err := o.FindFastest(ctx)
	if err != nil {
		t.Fatalf("FindFastest failed: %v", err)
	}
	
	if server.Addr == "" {
		t.Error("FindFastest returned empty address")
	}
	if server.Latency <= 0 {
		t.Error("FindFastest returned zero or negative latency")
	}
}

func TestBenchmarkServers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	
	o := NewOptimizer()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	
	servers, err := o.BenchmarkServers(ctx)
	if err != nil {
		t.Fatalf("BenchmarkServers failed: %v", err)
	}
	
	if len(servers) == 0 {
		t.Fatal("No servers returned from benchmark")
	}
	
	// Verify sorting (lowest score first)
	for i := 1; i < len(servers); i++ {
		if servers[i].Score < servers[i-1].Score {
			t.Errorf("Servers not sorted by score: %f > %f", servers[i-1].Score, servers[i].Score)
		}
	}
	
	// Verify all servers have addresses
	for _, s := range servers {
		if s.Addr == "" {
			t.Error("Server returned with empty address")
		}
	}
}

func BenchmarkFindFastest(b *testing.B) {
	o := NewOptimizer()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o.FindFastest(ctx)
	}
}

func BenchmarkBenchmarkServers(b *testing.B) {
	o := NewOptimizer()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o.BenchmarkServers(ctx)
	}
}
