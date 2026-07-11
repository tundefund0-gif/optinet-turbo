package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/optinet/internal/benchmark"
	"github.com/user/optinet/internal/dashboard"
	"github.com/user/optinet/internal/dns"
	"github.com/user/optinet/internal/latency"
	"github.com/user/optinet/internal/monitor"
	"github.com/user/optinet/internal/pool"
	"github.com/user/optinet/internal/proxy"
	"github.com/user/optinet/internal/tcpopt"
	"github.com/user/optinet/internal/udpproxy"
	"github.com/user/optinet/internal/hotspot"
)

var (
	proxyAddr     = "8080"
	dashboardAddr = "9090"
	tunnelAddr    = "1080"
)

func init() {
	if v := os.Getenv("OPTINET_PROXY_ADDR"); v != "" {
		proxyAddr = v
	}
	if v := os.Getenv("OPTINET_DASHBOARD_ADDR"); v != "" {
		dashboardAddr = v
	}
}

func main() {
	fmt.Println(`
  ╔═══════════════════════════════════════════╗
  ║         OptiNet v2.0 — Turbo Edition       ║
  ║     High-Performance Network Optimizer     ║
  ║           College Project                  ║
  ╚═══════════════════════════════════════════╝
	`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize components
	bufferPool := pool.NewBufferPool()
	dnsOpt := dns.NewOptimizer()
	latencyTester := latency.NewTester()
	benchSuite := benchmark.NewSuite()
	mon := monitor.NewMonitor(120)
	dashServer := dashboard.NewServer(":"+dashboardAddr, mon)
	tcpOptions := tcpopt.AggressiveOptions()

	// Create proxy servers
	proxyServer := proxy.NewServer(":" + proxyAddr)
	socksServer := proxy.NewSOCKS5Server(":" + tunnelAddr)

	// Find fastest DNS server
	findFastestDNS(ctx, dnsOpt, proxyServer)

	// Start HTTP proxy
	go func() {
		log.Printf("[Proxy] Starting HTTP proxy on :%s", proxyAddr)
		log.Printf("[Proxy] Configure phone WiFi proxy -> localhost:%s", proxyAddr)
		if err := proxyServer.Start(ctx); err != nil {
			log.Printf("[Proxy] Error: %v", err)
		}
	}()

	// Start SOCKS5 proxy
	go func() {
		log.Printf("[SOCKS5] Starting SOCKS5 proxy on :%s", tunnelAddr)
		if err := socksServer.Start(ctx); err != nil {
			log.Printf("[SOCKS5] Error: %v", err)
		}
	}()

		// Start UDP proxy (for gaming traffic)
	go func() {
		udpServer := udpproxy.NewServer(":5353", proxyServer.GetDNSServer())
		log.Printf("[UDP] Starting UDP proxy on :5353 (game traffic)")
		if err := udpServer.Start(ctx); err != nil {
			log.Printf("[UDP] Error: %v", err)
		}
	}()

// Start dashboard
	go func() {
		log.Printf("[Dashboard] Starting web UI on http://:%s", dashboardAddr)
		if err := dashServer.Start(ctx); err != nil {
			log.Printf("[Dashboard] Error: %v", err)
		}
	}()

		// Start hotspot
	hs := hotspot.NewManager()
	go func() {
		time.Sleep(1 * time.Second)
		log.Printf("[Hotspot] 🛜 Initializing secured hotspot...")
		if err := hs.Start(); err != nil {
			log.Printf("[Hotspot] Info: %v", err)
		}
	}()

	// Register hotspot with dashboard
	time.Sleep(100 * time.Millisecond)
	dashServer.SetHotspot(hs)
// Start monitoring
	go monitoringLoop(ctx, mon, latencyTester)

	// Start benchmark API
	go startBenchmarkAPI(ctx, benchSuite)

	// Run initial benchmark
	go func() {
		time.Sleep(2 * time.Second)
		log.Println("[Benchmark] Running initial network benchmark...")
		result, err := benchSuite.RunAll(ctx)
		if err != nil {
			log.Printf("[Benchmark] Error: %v", err)
			return
		}
		log.Printf("[Benchmark] Network Score: %.1f/100", result.OptimizationScore)
		log.Printf("[Benchmark] DNS: %s (%.2f ms)", result.DNSBenchmark.FastestServer, result.DNSBenchmark.FastestLatencyMs)
		log.Printf("[Benchmark] Avg Latency: %.1f ms, Jitter: %.1f ms",
			result.LatencyBenchmark.AvgLatencyMs,
			result.LatencyBenchmark.JitterMs)
	}()

	// Summary
	localIP := getLocalIP()
	log.Println("═══════════════════════════════════════════")
	log.Printf("  Dashboard:  http://%s:%s", localIP, dashboardAddr)
	log.Printf("  HTTP Proxy: %s:%s", localIP, proxyAddr)
	log.Printf("  SOCKS5:     %s:%s", localIP, tunnelAddr)
	log.Println("═══════════════════════════════════════════")
	_ = bufferPool
	_ = tcpOptions

	// Display hotspot info after startup
	go func() {
		time.Sleep(3 * time.Second)
		fmt.Print(hs.DisplayInfo())
	}()
	// Wait for shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	cancel()
	time.Sleep(500 * time.Millisecond)
	log.Println("Goodbye!")
}

func findFastestDNS(ctx context.Context, dnsOpt *dns.Optimizer, proxyServer *proxy.Server) {
	go func() {
		log.Println("[DNS] Finding fastest DNS server...")
		servers, err := dnsOpt.BenchmarkServers(ctx)
		if err != nil {
			log.Printf("[DNS] Error: %v (using default)", err)
			proxyServer.SetDNSServer("1.1.1.1:53")
			return
		}
		if len(servers) > 0 {
			fastest := servers[0]
			log.Printf("[DNS] Fastest: %s (%.2f ms, loss: %.1f%%)",
				fastest.Addr,
				float64(fastest.Latency.Microseconds())/1000.0,
				fastest.Loss)
			proxyServer.SetDNSServer(fastest.Addr)

			if len(servers) > 1 {
				worst := servers[len(servers)-1]
				improvement := float64(worst.Latency.Microseconds()-fastest.Latency.Microseconds()) / 1000.0
				log.Printf("[DNS] Improvement: %.2f ms over %s", improvement, worst.Addr)
			}
		}
	}()
}

func monitoringLoop(ctx context.Context, mon *monitor.Monitor, latencyTester *latency.Tester) {
	targets := []string{
		"8.8.8.8:443",
		"1.1.1.1:443",
		"9.9.9.9:443",
		"208.67.222.222:443",
	}

	// Initial measurement
	measureLatency(ctx, mon, latencyTester, targets)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			measureLatency(ctx, mon, latencyTester, targets)
		}
	}
}

func measureLatency(ctx context.Context, mon *monitor.Monitor, tester *latency.Tester, targets []string) {
	results, err := tester.BenchmarkTargets(ctx, targets)
	if err != nil || len(results) == 0 {
		return
	}

	var totalLat float64
	var count int
	var latencyValues []float64

	for _, r := range results {
		if r.Reachable {
			lat := float64(r.Stats.Avg.Microseconds()) / 1000.0
			totalLat += lat
			count++
			latencyValues = append(latencyValues, lat)
		}
	}

	if count == 0 {
		return
	}

	avgLat := totalLat / float64(count)
	jitter := monitor.CalculateJitter(latencyValues)

	// Calculate packet loss
	var totalSamples, totalLost int
	for _, r := range results {
		totalSamples += r.Stats.Samples
		totalLost += r.Stats.Lost
	}
	loss := 0.0
	if totalSamples > 0 {
		loss = float64(totalLost) / float64(totalSamples) * 100
	}

	// DNS speed estimate (will be refined by benchmark)
	dnsSpeed := 15.0

	mon.Update(avgLat, jitter, math.Round(loss*100)/100, dnsSpeed)
}

func startBenchmarkAPI(ctx context.Context, benchSuite *benchmark.Suite) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/benchmark", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		if r.Method == "POST" {
			log.Println("[API] Running benchmark (triggered by user)...")
			result, err := benchSuite.RunAll(ctx)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": err.Error(),
				})
				return
			}
			json.NewEncoder(w).Encode(result)
			return
		}

		// Quick measurement
		qr, _ := benchmark.QuickBenchmark(ctx)
		if qr != nil {
			json.NewEncoder(w).Encode(qr)
		}
	})

	server := &http.Server{
		Addr:    ":9091",
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Printf("[API] Benchmark API on :9091")
	server.ListenAndServe()
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "localhost"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
