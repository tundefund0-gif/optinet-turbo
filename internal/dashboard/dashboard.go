package dashboard

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net"
	"sync"
	"time"

	"github.com/user/optinet/internal/monitor"
)

//go:embed index.html
var dashboardHTML string

type Server struct {
	addr         string
	monitor      *monitor.Monitor
	dnsLatency   float64
	mu           sync.RWMutex
	proxyStatus  string
	serverStatus string
}

func NewServer(addr string, mon *monitor.Monitor) *Server {
	return &Server{
		addr:         addr,
		monitor:      mon,
		proxyStatus:  "off",
		serverStatus: "running",
	}
}

func (ds *Server) SetDNSLatency(latency float64) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.dnsLatency = latency
}

func (ds *Server) SetProxyStatus(status string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.proxyStatus = status
}

func (ds *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ds.handleDashboard)
	mux.HandleFunc("/api/metrics", ds.handleMetrics)
	mux.HandleFunc("/api/dns-servers", ds.handleDNSServers)
	mux.HandleFunc("/api/status", ds.handleStatus)
	mux.HandleFunc("/api/game-servers", ds.handleGameServers)

	server := &http.Server{Addr: ds.addr, Handler: mux}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Printf("[Dashboard] Web UI at http://%s", ds.addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (ds *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	metrics := ds.monitor.GetMetrics()

	data := map[string]interface{}{
		"Latency":     fmt.Sprintf("%.1f", metrics.Latency),
		"LatencyMax":  fmt.Sprintf("%.1f", metrics.LatencyMax),
		"LatencyMin":  fmt.Sprintf("%.1f", metrics.LatencyMin),
		"Jitter":      fmt.Sprintf("%.1f", metrics.Jitter),
		"PacketLoss":  fmt.Sprintf("%.1f", metrics.PacketLoss),
		"Connections": metrics.Connections,
		"UpSpeed":     fmt.Sprintf("%.1f", metrics.UpSpeed),
		"DownSpeed":   fmt.Sprintf("%.1f", metrics.DownSpeed),
	}

	ds.mu.RLock()
	data["ProxyStatus"] = ds.proxyStatus
	data["DNSLatency"] = fmt.Sprintf("%.1f", ds.dnsLatency)
	ds.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html")
	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))
	tmpl.Execute(w, data)
}

func (ds *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := ds.monitor.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(metrics)
}

func (ds *Server) handleDNSServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	servers := []map[string]interface{}{
		{"name": "Cloudflare", "addr": "1.1.1.1", "latency": "—"},
		{"name": "Google", "addr": "8.8.8.8", "latency": "—"},
		{"name": "Quad9", "addr": "9.9.9.9", "latency": "—"},
		{"name": "OpenDNS", "addr": "208.67.222.222", "latency": "—"},
	}
	json.NewEncoder(w).Encode(servers)
}

func (ds *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ds.mu.RLock()
	status := map[string]interface{}{
		"proxy":   ds.proxyStatus,
		"server":  ds.serverStatus,
		"uptime":  time.Now().Unix(),
	}
	ds.mu.RUnlock()
	json.NewEncoder(w).Encode(status)
}

func (ds *Server) handleGameServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	targets := []struct {
		Name string
		Addr string
	}{
		{"Google", "8.8.8.8:443"},
		{"Cloudflare", "1.1.1.1:443"},
		{"Quad9", "9.9.9.9:443"},
		{"OpenDNS", "208.67.222.222:443"},
	}

	type serverStatus struct {
		Name    string  `json:"name"`
		Addr    string  `json:"addr"`
		Latency float64 `json:"latency"`
		Loss    float64 `json:"loss"`
	}

	results := make([]serverStatus, 0, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, t := range targets {
		wg.Add(1)
		go func(name, addr string) {
			defer wg.Done()
			start := time.Now()
			d := net.Dialer{Timeout: 2 * time.Second}
			conn, err := d.DialContext(ctx, "tcp", addr)
			if err != nil {
				mu.Lock()
				results = append(results, serverStatus{Name: name, Addr: addr, Latency: 0, Loss: 100})
				mu.Unlock()
				return
			}
			latency := float64(time.Since(start).Microseconds()) / 1000.0
			conn.Close()
			mu.Lock()
			results = append(results, serverStatus{Name: name, Addr: addr, Latency: latency, Loss: 0})
			mu.Unlock()
		}(t.Name, t.Addr)
	}

	wg.Wait()
	json.NewEncoder(w).Encode(results)
}
