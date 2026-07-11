# OptiNet — Smart Network Optimizer

> **College Project** — Optimize your network, lower your ping, boost your game score.

## How It Works

```
┌───────────────────────────────────────────┐
│         Your Android (Termux)              │
│                                           │
│  ┌─────────────┐    ┌──────────────────┐  │
│  │ OptiNet     │───▶│ Web Dashboard     │  │
│  │ Go Server   │    │ (Browser @:9090)  │  │
│  │             │    └──────────────────┘  │
│  │ • DNS Opt   │                          │
│  │ • Latency   │───▶ Proxy / VPN Overlay  │
│  │ • Monitor   │    (HTTP :8080 / SOCKS5  │
│  └─────────────┘     :1080)               │
└───────────────────────────────────────────┘
         │
         ▼ Your game traffic gets optimized
```

### What it does:
- **Smart DNS** — Finds fastest DNS server (Cloudflare, Google, Quad9...)
- **Latency Monitor** — Pings game/CDN servers, tracks real-time ping
- **HTTP Proxy** — Route your phone's WiFi through OptiNet for optimization
- **SOCKS5 Proxy** — For apps that support SOCKS (works with Drony/Postern for full VPN overlay)
- **Live Dashboard** — Beautiful real-time charts of your network performance
- **VPN Overlay Toggle** — Turn optimization on/off from the dashboard

## Setup on Your Phone (No Root!)

### Step 1: Install Termux
Download from **F-Droid** (NOT Play Store — Play Store version is outdated):
https://f-droid.org/packages/com.termux/

### Step 2: Install Go & Build
```bash
pkg update && pkg upgrade -y
pkg install golang git -y

# Copy the optinet folder to your phone, then:
cd optinet
go build -o optinetd ./cmd/optinetd
```

### Step 3: Run It
```bash
./optinetd
```

### Step 4: Open Dashboard
Open your phone's browser to: **http://localhost:9090**

### Step 5: Connect via Proxy

**Option A — WiFi Proxy (Easy):**
- Settings → WiFi → Long-press your network → Modify network
- Advanced options → Proxy → Manual
- Host: `localhost`  Port: `8080`
- Only HTTP traffic goes through the optimizer

**Option B — VPN Overlay (Full traffic, recommended):**
- Install **Drony** or **Postern** from Play Store
- Configure it to route **all traffic** through SOCKS5 proxy at `localhost:1080`
- Every app's traffic gets optimized!

## Features

| Feature | Description |
|---------|-------------|
| DNS Optimizer | Benchmarks 5 major DNS servers, uses the fastest |
| Latency Tester | Pings game/CDN servers every 3 seconds |
| Real-time Charts | Live latency graph on the dashboard |
| HTTP Proxy | Port 8080 — standard HTTP proxy for browsers |
| SOCKS5 Proxy | Port 1080 — full TCP proxy for VPN overlay |
| Jitter Monitor | Tracks connection stability |
| Speed Display | Shows simulated up/down speeds |

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `/` | Web dashboard |
| `/api/metrics` | JSON latency/jitter/packet loss data |
| `/api/game-servers` | Game server ping list |
| `/api/dns-servers` | DNS server benchmark results |
| `/api/status` | Server & proxy status |

## Tech Stack

- **Go** — Core networking, concurrency, HTTP/SOCKS5 servers
- **Embed** — Go 1.16+ embed for static assets
- **Chart.js** — Real-time latency charts
- **HTML/CSS** — Mobile-first dark UI

## Project Structure

```
optinet/
├── cmd/optinetd/main.go       # Entry point
├── internal/
│   ├── dns/dns.go             # DNS optimizer
│   ├── latency/latency.go     # Latency tester
│   ├── proxy/proxy.go         # HTTP + SOCKS5 proxy
│   ├── monitor/monitor.go     # Network metrics collector
│   └── dashboard/dashboard.go # Web dashboard server
├── web/index.html             # Dashboard HTML template
├── Makefile                   # Build + setup targets
├── setup_termux.sh            # Termux setup script
└── README.md
```

## Why This Rocks for a College Project

1. **Real networking concepts** — DNS, TCP, proxies, SOCKS5, latency, jitter
2. **Go concurrency** — Goroutines for parallel DNS testing, proxy connections
3. **Full-stack** — Go backend + HTML/CSS/JS frontend
4. **Works on real hardware** — Your actual phone, no emulator
5. **Visually impressive** — Live charts, mobile UI, real-time updates
6. **Practical problem** — Network optimization everyone understands

## License

MIT — Do whatever you want, this is a college project.
