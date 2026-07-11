package hotspot

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Status string
const (
	StatusOff      Status = "off"
	StatusStarting Status = "starting"
	StatusOn       Status = "on"
	StatusError    Status = "error"
)

type Config struct {
	SSID     string `json:"ssid"`
	Password string `json:"password"`
	Band     string `json:"band"`
}

type Manager struct {
	config      Config
	status      Status
	mu          sync.RWMutex
	clientIP    string
	startMethod string
}

func NewManager() *Manager {
	return &Manager{
		config: Config{
			SSID:     generateSSID(),
			Password: generatePassword(12),
			Band:     "2.4GHz",
		},
		status: StatusOff,
	}
}

func (m *Manager) GetConfig() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) GetClientIP() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clientIP
}

func (m *Manager) GetStartMethod() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.startMethod
}

func (m *Manager) Start() error {
	m.mu.Lock()
	m.status = StatusStarting
	cfg := m.config
	m.mu.Unlock()

	log.Printf("[Hotspot] Creating secured network...")
	log.Printf("[Hotspot] SSID:     %s", cfg.SSID)
	log.Printf("[Hotspot] Password: %s", cfg.Password)

	if err := exec.Command("termux-wifi-hotspot", cfg.SSID, cfg.Password).Run(); err == nil {
		m.mu.Lock()
		m.status = StatusOn
		m.startMethod = "termux-api"
		m.mu.Unlock()
		log.Printf("[Hotspot] Hotspot created automatically!")
		go m.monitorConnection()
		return nil
	}

	log.Printf("[Hotspot] Opening hotspot settings...")
	intent := "am start -a android.settings.TETHER_SETTINGS"
	if err := exec.Command("sh", "-c", intent).Run(); err == nil {
		m.mu.Lock()
		m.status = StatusStarting
		m.startMethod = "settings"
		m.mu.Unlock()
		log.Printf("[Hotspot] Turn ON hotspot in settings now!")
		go m.waitForHotspot(cfg)
	} else {
		m.mu.Lock()
		m.status = StatusOff
		m.startMethod = "manual"
		m.mu.Unlock()
		log.Printf("[Hotspot] Enable hotspot manually on this phone")
	}

	return nil
}

func (m *Manager) waitForHotspot(cfg Config) {
	for i := 0; i < 30; i++ {
		time.Sleep(2 * time.Second)
		if m.isHotspotActive() {
			m.mu.Lock()
			m.status = StatusOn
			m.mu.Unlock()
			log.Printf("[Hotspot] Hotspot is active!")
			log.Printf("[Hotspot] Connect to: %s (password: %s)", cfg.SSID, cfg.Password)
			go m.monitorConnection()
			return
		}
		if i%5 == 0 && i > 0 {
			log.Printf("[Hotspot] Waiting for hotspot... (%ds)", i*2)
		}
	}
	m.mu.Lock()
	m.status = StatusOff
	m.mu.Unlock()
	log.Printf("[Hotspot] Timeout. Enable hotspot manually.")
}

func (m *Manager) isHotspotActive() bool {
	out, err := exec.Command("termux-wifi-connectioninfo").Output()
	if err == nil && (strings.Contains(string(out), "softap") || strings.Contains(string(out), "hotspot")) {
		return true
	}
	_, err = net.DialTimeout("tcp", "192.168.43.1:9090", 500*time.Millisecond)
	return err == nil
}

func (m *Manager) monitorConnection() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		m.mu.RLock()
		if m.status != StatusOn { m.mu.RUnlock(); return }
		m.mu.RUnlock()
		clients := getConnectedClients()
		if len(clients) > 0 {
			m.mu.Lock()
			m.clientIP = clients[0]
			m.mu.Unlock()
		}
	}
}

func (m *Manager) DisplayInfo() string {
	cfg := m.GetConfig()
	status := m.GetStatus()
	method := m.GetStartMethod()
	clientIP := m.GetClientIP()

	s := "\n"
	s += "  ================================\n"
	s += "    OptiNet Secured Hotspot\n"
	s += "  ================================\n"
	s += fmt.Sprintf("  Status:   %s\n", status)
	s += fmt.Sprintf("  SSID:     %s\n", cfg.SSID)
	s += fmt.Sprintf("  Password: %s\n", cfg.Password)
	s += fmt.Sprintf("  Band:     %s\n", cfg.Band)
	s += fmt.Sprintf("  Mode:     %s\n", method)
	if clientIP != "" {
		s += fmt.Sprintf("  Client:   %s\n", clientIP)
	}
	s += "\n"
	s += "  1. Connect to: " + cfg.SSID + "\n"
	s += "     Password:   " + cfg.Password + "\n"
	s += "\n"
	s += "  2. Set SOCKS5 proxy to 192.168.43.1:1080\n"
	s += "\n"
	s += "  3. Dashboard: http://192.168.43.1:9090\n"
	return s
}

func generateSSID() string {
	b := make([]byte, 3)
	rand.Read(b)
	return "OptiNet-" + strings.ToUpper(hex.EncodeToString(b))
}

func generatePassword(length int) string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	pw := make([]byte, length)
	for i := range pw {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		pw[i] = charset[n.Int64()]
	}
	return string(pw)
}

func getConnectedClients() []string {
	var clients []string
	out, err := exec.Command("ip", "neigh").Output()
	if err != nil {
		return clients
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[4] == "REACHABLE" {
			ip := net.ParseIP(fields[0])
			if ip != nil && ip.IsPrivate() {
				clients = append(clients, fields[0])
			}
		}
	}
	return clients
}
