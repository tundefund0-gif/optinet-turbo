# OptiNet Turbo — Smart Network Optimizer

> **College Project** — Optimize your network, lower your ping, boost your game score.  
> High-performance DNS optimizer + HTTP/SOCKS5 proxy with live dashboard.

```
  ╔═══════════════════════════════════════════╗
  ║         OptiNet v2.0 — Turbo Edition       ║
  ║     High-Performance Network Optimizer     ║
  ║           College Project                  ║
  ╚═══════════════════════════════════════════╝
```

---

## Architecture

```
┌─────────────────────────┐      Hotspot/WiFi       ┌──────────────────────┐
│   SERVER PHONE (Termux) │◄──────────────────────►│   CLIENT PHONE       │
│                         │                         │   (Gaming Phone)     │
│  ┌───────────────────┐  │   SOCKS5 :1080          │                      │
│  │   OptiNet Turbo   │  │   HTTP   :8080          │  ┌────────────────┐  │
│  │                    │──┤──────────────────────────┤  Super Proxy    │  │
│  │  • DNS Optimizer  │  │                         │  │ or Drony       │  │
│  │  • Latency Monitor│  │   Dashboard :9090       │  │ (SOCKS5 client)│  │
│  │  • HTTP Proxy     │  │   Bench API :9091       │  └────────────────┘  │
│  │  • SOCKS5 Proxy   │  │   UDP Game  :5353       │                      │
│  │  • Live Dashboard │  │                         │  Or WiFi Proxy       │
│  └───────────────────┘  │                         │  HTTP :8080          │
└─────────────────────────┘                         └──────────────────────┘
```

---

## What It Does

- **Smart DNS** — Benchmarks 5 major DNS servers (Google, Cloudflare, Quad9, etc.) and auto-selects the fastest
- **Latency Monitor** — Pings game/CDN servers every 3 seconds, tracks real-time ping + jitter
- **HTTP Proxy** (`:8080`) — Route your phone's WiFi proxy through OptiNet for HTTP optimization
- **SOCKS5 Proxy** (`:1080`) — Full TCP proxy for apps that support SOCKS (gaming, streaming, etc.)
- **UDP Game Proxy** (`:5353`) — UDP traffic forwarding for gaming
- **Live Dashboard** (`:9090`) — Real-time charts of latency, DNS speed, connection stats
- **Benchmark API** (`:9091`) — Run network benchmarks on demand
- **Network Scoring** — Overall network quality score out of 100

---

## Server Setup (Remote Phone — Termux)

### Prerequisites
- Android phone with **Termux** installed
- **Go** installed: `pkg install golang`
- Phone is hosting a **WiFi hotspot** or on same network as client

### Step 1: Install & Build
```bash
# Update packages
pkg update && pkg upgrade -y
pkg install golang git -y

# Clone the repo
git clone https://github.com/tundefund0-gif/optinet-turbo.git
cd optinet-turbo

# Build
go build -o optinetd ./cmd/optinetd
```

### Step 2: Start the Server
```bash
# Run in foreground (for testing)
./optinetd

# Or run in background
nohup ./optinetd > optinet.log 2>&1 &

# Or with tmux (recommended)
tmux new-session -d -s optinet './optinetd'
```

### Step 3: Check It's Running
```bash
# View logs
cat optinet.log

# You should see:
#   Dashboard:  http://192.168.x.x:9090
#   HTTP Proxy: 192.168.x.x:8080
#   SOCKS5:     192.168.x.x:1080

# Test locally
curl --socks5-hostname 127.0.0.1:1080 -s -o /dev/null -w '%{http_code}' http://google.com
# Should return 200 or 301
```

---

## Client Phone Setup (Your Gaming Phone)

Your gaming phone connects to the **server phone's hotspot** and routes traffic through OptiNet.

### Step 1: Connect to Hotspot
- Connect your gaming phone to the server phone's **WiFi hotspot**
- Note the server's IP address (e.g. `192.168.218.187`)

### Step 2: Choose Your Proxy App

#### Option A: Super Proxy (Simplest)
1. Install **Super Proxy** from Play Store
2. Open → tap **+**
3. Enter:
   - **Type**: `SOCKS5`
   - **Host**: `192.168.218.187` (your server's hotspot IP)
   - **Port**: `1080`
4. Save → tap **Connect**
5. Verify at: `http://192.168.218.187:9090`

#### Option B: Drony (Per-app Routing)
1. Install **Drony** from Play Store
2. Open → **Settings** → **Network** → **WiFi**
3. Select your hotspot → **Manual proxy**
4. Enter:
   - **Host**: `192.168.218.187`
   - **Port**: `1080`
   - **Type**: `SOCKS5`
5. Back → tap **Start** (red icon turns green)

#### Option C: Manual WiFi Proxy (HTTP only)
- WiFi Settings → Long-press network → Modify network
- Advanced → Proxy → **Manual**
- Host: `192.168.218.187`
- Port: `8080`
*(Note: Only HTTP traffic routes through — some apps won't work)*

---

## Dashboard

Open in any browser: **http://192.168.218.187:9090**

**What you'll see:**
- **Network Score** — Overall quality out of 100
- **Latency Graph** — Real-time ping chart
- **DNS Status** — Fastest detected DNS server
- **Live Stats** — Active connections, throughput, uptime
- **Jitter Monitor** — Connection stability tracking

**API endpoints:**
| Endpoint | Description |
|----------|-------------|
| `/api/metrics` | JSON latency/jitter/packet loss |
| `/api/game-servers` | Game server ping list |
| `/api/dns-servers` | DNS benchmark results |
| `/api/status` | Server & proxy status |
| `/api/benchmark` | Run speed benchmark |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPTINET_PROXY_ADDR` | `8080` | HTTP proxy port |
| `OPTINET_DASHBOARD_ADDR` | `9090` | Dashboard web UI port |

No config file needed — just set env vars:
```bash
OPTINET_PROXY_ADDR=8080 OPTINET_DASHBOARD_ADDR=9090 ./optinetd
```

---

## Building for Different Architectures

```bash
# Build for current system
go build -o optinetd ./cmd/optinetd

# Cross-compile for ARM32 (most Android phones)
GOOS=linux GOARCH=arm GOARM=7 go build -o optinetd_arm ./cmd/optinetd

# Cross-compile for ARM64
GOOS=linux GOARCH=arm64 go build -o optinetd_arm64 ./cmd/optinetd

# Cross-compile for x86_64
GOOS=linux GOARCH=amd64 go build -o optinetd_amd64 ./cmd/optinetd
```

---

## Project Structure

```
optinet/
├── cmd/optinetd/
│   └── main.go              # Entry point, orchestrates all services
├── internal/
│   ├── benchmark/
│   │   └── benchmark.go     # Network scoring engine
│   ├── dashboard/
│   │   └── dashboard.go     # Web dashboard + live charts
│   ├── dns/
│   │   ├── dns.go           # DNS optimizer (benchmarks & selects fastest)
│   │   └── dns_test.go
│   ├── latency/
│   │   ├── latency.go       # ICMP/TCP latency tester
│   │   └── latency_test.go
│   ├── monitor/
│   │   ├── monitor.go       # Network metric collector
│   │   └── monitor_test.go
│   ├── pool/
│   │   ├── pool.go          # Buffer pool for zero-copy relay
│   │   └── pool_test.go
│   ├── proxy/
│   │   └── proxy.go         # HTTP + SOCKS5 proxy servers
│   ├── speedtest/
│   │   ├── speedtest.go     # Bandwidth measurement
│   │   └── speedtest_test.go
│   ├── tcpopt/
│   │   ├── tcpopt.go        # TCP kernel optimizations
│   │   └── tcpopt_test.go
│   ├── udpproxy/
│   │   └── udpproxy.go      # UDP game traffic proxy
│   └── workerpool/
│       └── workerpool.go    # Goroutine pool for concurrency
├── go.mod
└── README.md
```

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| Dashboard not loading | Check server: `ps aux \| grep optinetd` |
| "address already in use" | Change ports via env vars or kill old process: `pkill -f optinetd` |
| Super Proxy won't connect | Use **SOCKS5** type, not HTTP — port `1080` not `8080` |
| High latency improvements | Make sure both phones are on 5GHz hotspot |
| Connection drops | Keep server phone plugged in and screen on |
| Wrong IP shown | Use `OPTINET_PROXY_ADDR` and check `ifconfig` for actual hotspot IP |
| DNS errors | The server auto-selects fastest DNS — give it 5 seconds after start |

---

## Performance Tips

- **5GHz hotspot** gives lower latency than 2.4GHz
- **Keep server phone charging** — proxy drains battery
- **Close background apps** on both phones for more bandwidth
- **Check the dashboard** before gaming — aim for Network Score > 70
- **Use SOCKS5** (port 1080) instead of HTTP proxy for full traffic routing

---

## Why This Rocks for a College Project

1. **Real networking** — DNS, TCP, SOCKS5, latency, jitter, packet loss
2. **Go concurrency** — Goroutines for parallel DNS testing, proxy connections
3. **Full-stack** — Go backend + HTML/CSS/JS frontend with live charts
4. **Works on real phones** — No emulator, actual hardware
5. **Visually impressive** — Live dashboard, real-time metrics
6. **Practical problem** — Network optimization that everyone understands
7. **Benchmark scoring** — Quantifiable results (Network Score /100)

---

## License

MIT — College Project
