package udpproxy

import (
	"context"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Stats tracks UDP proxy performance
type Stats struct {
	PacketsIn    int64 `json:"packets_in"`
	PacketsOut   int64 `json:"packets_out"`
	BytesIn      int64 `json:"bytes_in"`
	BytesOut     int64 `json:"bytes_out"`
	Sessions     int64 `json:"sessions"`
	StartTime    int64 `json:"start_time"`
	ConnsActive  int32 `json:"conns_active"`
}

// Session tracks a UDP session
type Session struct {
	ClientAddr *net.UDPAddr
	TargetConn *net.UDPConn
	TargetAddr *net.UDPAddr
	LastUsed   time.Time
}

// Server is a UDP proxy with DNS and game traffic forwarding
type Server struct {
	addr        string
	stats       Stats
	conn        *net.UDPConn
	sessions    map[string]*Session
	mu          sync.RWMutex
	dnsAddr     string
	sessionTTL  time.Duration
}

// NewServer creates a UDP proxy
func NewServer(addr string, dnsAddr string) *Server {
	if dnsAddr == "" {
		dnsAddr = "1.1.1.1:53"
	}
	return &Server{
		addr:       addr,
		dnsAddr:    dnsAddr,
		sessions:   make(map[string]*Session),
		sessionTTL: 2 * time.Minute,
		stats: Stats{
			StartTime: time.Now().Unix(),
		},
	}
}

// GetStats returns stats
func (s *Server) GetStats() Stats {
	return Stats{
		PacketsIn:   atomic.LoadInt64(&s.stats.PacketsIn),
		PacketsOut:  atomic.LoadInt64(&s.stats.PacketsOut),
		BytesIn:     atomic.LoadInt64(&s.stats.BytesIn),
		BytesOut:    atomic.LoadInt64(&s.stats.BytesOut),
		Sessions:    atomic.LoadInt64(&s.stats.Sessions),
		ConnsActive: int32(len(s.sessions)),
		StartTime:   atomic.LoadInt64(&s.stats.StartTime),
	}
}

// SetDNSAddr updates the DNS server address
func (s *Server) SetDNSAddr(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnsAddr = addr
}

// Start begins the UDP proxy
func (s *Server) Start(ctx context.Context) error {
	pc, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		return err
	}
	s.conn = pc.(*net.UDPConn)

	log.Printf("[UDP] Listening on %s (DNS→%s)", s.addr, s.dnsAddr)

	go s.cleanupLoop(ctx)

	buf := make([]byte, 65507)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		s.conn.SetDeadline(time.Now().Add(2 * time.Second))
		n, clientAddr, err := s.conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-ctx.Done():
				return nil
			default:
				continue
			}
		}

		data := make([]byte, n)
		copy(data, buf[:n])
		go s.handlePacket(data, clientAddr.(*net.UDPAddr))
	}
}

func (s *Server) handlePacket(data []byte, clientAddr *net.UDPAddr) {
	key := clientAddr.String()

	// Check if we have an existing session
	s.mu.RLock()
	session, exists := s.sessions[key]
	s.mu.RUnlock()

	if exists {
		_, err := session.TargetConn.Write(data)
		if err != nil {
			s.closeSession(key)
			return
		}
		atomic.AddInt64(&s.stats.PacketsOut, 1)
		atomic.AddInt64(&s.stats.BytesOut, int64(len(data)))
		session.LastUsed = time.Now()
		return
	}

	// Determine target based on port
	var targetAddr *net.UDPAddr

	// If it looks like DNS (port follows DNS format), route to optimized DNS
	if isDNSQuery(data) {
		targetAddr, _ = net.ResolveUDPAddr("udp", s.dnsAddr)
	} else {
		// For non-DNS UDP traffic, forward to common game/CDN targets
		// In a full implementation, this would use SOCKS5 UDP ASSOCIATE
		// For now, forward based on destination port heuristics
		targetAddr = resolveTarget(clientAddr, data)
	}

	if targetAddr == nil {
		return // Can't determine target
	}

	targetConn, err := net.DialUDP("udp", nil, targetAddr)
	if err != nil {
		return
	}

	_, err = targetConn.Write(data)
	if err != nil {
		targetConn.Close()
		return
	}

	session = &Session{
		ClientAddr: clientAddr,
		TargetConn: targetConn,
		TargetAddr: targetAddr,
		LastUsed:   time.Now(),
	}

	s.mu.Lock()
	s.sessions[key] = session
	atomic.AddInt64(&s.stats.Sessions, 1)
	s.mu.Unlock()

	atomic.AddInt64(&s.stats.PacketsOut, 1)
	atomic.AddInt64(&s.stats.BytesOut, int64(len(data)))

	go s.relayResponses(session, key)
}

func (s *Server) relayResponses(session *Session, key string) {
	buf := make([]byte, 65507)
	for {
		session.TargetConn.SetDeadline(time.Now().Add(s.sessionTTL))
		n, err := session.TargetConn.Read(buf)
		if err != nil {
			s.closeSession(key)
			return
		}
		_, err = s.conn.WriteTo(buf[:n], session.ClientAddr)
		if err != nil {
			s.closeSession(key)
			return
		}
		atomic.AddInt64(&s.stats.PacketsIn, 1)
		atomic.AddInt64(&s.stats.BytesIn, int64(n))
		session.LastUsed = time.Now()
	}
}

func (s *Server) closeSession(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[key]; ok {
		session.TargetConn.Close()
		delete(s.sessions, key)
	}
}

func (s *Server) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for key, session := range s.sessions {
				if now.Sub(session.LastUsed) > s.sessionTTL {
					session.TargetConn.Close()
					delete(s.sessions, key)
				}
			}
			s.mu.Unlock()
		}
	}
}

// isDNSQuery checks if packet looks like a DNS query
func isDNSQuery(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	// DNS: first bit of byte 2 is 0 for query, byte 3 has flags
	// Simple check: QR bit (bit 15) should be 0 for query
	return data[2]&0x80 == 0 && len(data) >= 12
}

// resolveTarget tries to find destination for non-DNS UDP traffic
func resolveTarget(clientAddr *net.UDPAddr, data []byte) *net.UDPAddr {
	// For game traffic, forward to common CDN/game ports
	// This is a simplified approach - real implementation would use
	// SOCKS5 UDP ASSOCIATE or parse the packet's original destination
	port := clientAddr.Port

	// Common game-related ports that need UDP forwarding
	switch {
	case port >= 27000 && port <= 27100:
		// Steam game ports - forward to local upstream
		// In transparent mode, we'd route to original destination
		return nil // Would need actual target
	case port == 3478 || port == 3479:
		// STUN/TURN - forward to Google STUN
		addr, _ := net.ResolveUDPAddr("udp", "8.8.8.8:3478")
		return addr
	case port == 443:
		// QUIC/HTTP3 traffic
		return nil // Would need target mapping
	default:
		return nil
	}
}
