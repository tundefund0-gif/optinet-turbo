package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/optinet/internal/pool"
	"github.com/user/optinet/internal/tcpopt"
)

// Stats tracks proxy performance metrics
type Stats struct {
	BytesIn      int64   `json:"bytes_in"`
	BytesOut     int64   `json:"bytes_out"`
	ConnsTotal   int64   `json:"conns_total"`
	ConnsFailed  int64   `json:"conns_failed"`
	DNSQueries   int64   `json:"dns_queries"`
	LatencySum   int64   `json:"latency_sum_ns"`
	LatencyCount int64   `json:"latency_count"`
	StartTime    int64   `json:"start_time"`
	ConnsActive  int32   `json:"conns_active"`
	ThroughputIn  float64 `json:"throughput_in_mbps"`
	ThroughputOut float64 `json:"throughput_out_mbps"`
}

// AvgLatency returns the average connection latency
func (s *Stats) AvgLatency() time.Duration {
	if s.LatencyCount == 0 {
		return 0
	}
	return time.Duration(s.LatencySum / s.LatencyCount)
}

// Server is a high-performance HTTP/CONNECT/SOCKS5 proxy
type Server struct {
	lastBytesIn  int64
	lastBytesOut int64
	startTime    int64
	addr        string
	stats       Stats
	dnsServer   string
	mu          sync.RWMutex
	pool        *pool.BufferPool
	tcpOpts     *tcpopt.Options
	lastMeasTime time.Time
}

// NewServer creates a new high-performance proxy server
func NewServer(addr string) *Server {
	return &Server{
		addr:      addr,
		dnsServer: "1.1.1.1:53",
		pool:      pool.NewBufferPool(),
		tcpOpts:   tcpopt.AggressiveOptions(),
		startTime: time.Now().UnixNano(),
		stats: Stats{
			StartTime: time.Now().Unix(),
		},
		lastMeasTime: time.Now(),
	}
}

// SetDNSServer sets the DNS server for optimization
func (s *Server) SetDNSServer(dnsAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnsServer = dnsAddr
}

// GetDNSServer returns the current DNS server
func (s *Server) GetDNSServer() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dnsServer
}

// GetStats returns current proxy statistics
func (s *Server) GetStats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.lastMeasTime).Seconds()
	if elapsed >= 1.0 {
		bytesIn := atomic.LoadInt64(&s.stats.BytesIn)
		bytesOut := atomic.LoadInt64(&s.stats.BytesOut)
		s.stats.ThroughputIn = (float64(bytesIn-s.lastBytesIn) * 8) / (elapsed * 1000 * 1000)
		s.stats.ThroughputOut = (float64(bytesOut-s.lastBytesOut) * 8) / (elapsed * 1000 * 1000)
		s.lastBytesIn = bytesIn
		s.lastBytesOut = bytesOut
		s.lastMeasTime = now
	}

	return Stats{
		BytesIn:       atomic.LoadInt64(&s.stats.BytesIn),
		BytesOut:      atomic.LoadInt64(&s.stats.BytesOut),
		ConnsActive:   atomic.LoadInt32(&s.stats.ConnsActive),
		ConnsTotal:    atomic.LoadInt64(&s.stats.ConnsTotal),
		ConnsFailed:   atomic.LoadInt64(&s.stats.ConnsFailed),
		DNSQueries:    atomic.LoadInt64(&s.stats.DNSQueries),
		LatencySum:    atomic.LoadInt64(&s.stats.LatencySum),
		LatencyCount:  atomic.LoadInt64(&s.stats.LatencyCount),
		StartTime:     atomic.LoadInt64(&s.stats.StartTime),
		ThroughputIn:  s.stats.ThroughputIn,
		ThroughputOut: s.stats.ThroughputOut,
	}
}

// Start begins the proxy server
func (s *Server) Start(ctx context.Context) error {
	lc := net.ListenConfig{
		KeepAlive: 30 * time.Second,
	}
	listener, err := lc.Listen(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}
	log.Printf("[Proxy] Listening on %s", s.addr)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				continue
			}
		}

		atomic.AddInt32(&s.stats.ConnsActive, 1)
		atomic.AddInt64(&s.stats.ConnsTotal, 1)

		// Apply TCP optimizations
		tcpopt.Apply(conn, s.tcpOpts)

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(client net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Proxy] Recovered from panic: %v", r)
		}
	}()
	defer client.Close()
	defer atomic.AddInt32(&s.stats.ConnsActive, -1)

	client.SetDeadline(time.Now().Add(60 * time.Second))

	// Read the first few bytes to determine protocol
	buf := make([]byte, 3)
	n, err := io.ReadAtLeast(client, buf, 3)
	if err != nil {
		atomic.AddInt64(&s.stats.ConnsFailed, 1)
		return
	}

	// Create a reader that includes the peeked bytes
	reader := io.MultiReader(
		io.LimitReader(
			&bytesReader{data: buf[:n]},
			int64(n),
		),
		client,
	)

	// Determine protocol
	if buf[0] == 5 {
		// SOCKS5
		client.SetDeadline(time.Time{})
		s.handleSOCKS5FromReader(client, reader, buf[:n])
	} else if buf[0] == 'C' && n >= 3 && string(buf[:3]) == "CON" {
		// HTTP CONNECT
		client.SetDeadline(time.Time{})
		s.handleCONNECT(client, reader)
	} else {
		// Regular HTTP proxy
		client.SetDeadline(time.Time{})
		s.handleHTTP(client, buf[:n], reader)
	}
}

func (s *Server) handleCONNECT(client net.Conn, reader io.Reader) {
	br := bufio.NewReader(reader)
	req, err := http.ReadRequest(br)
	if err != nil {
		atomic.AddInt64(&s.stats.ConnsFailed, 1)
		return
	}

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	start := time.Now()

	// Connect to target with optimizations
	target, err := tcpopt.DialWithOpts("tcp", host, s.tcpOpts)
	if err != nil {
		log.Printf("[Proxy] CONNECT failed to %s: %v", host, err)
		atomic.AddInt64(&s.stats.ConnsFailed, 1)
		client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer target.Close()

	// Record latency
	atomic.AddInt64(&s.stats.LatencySum, time.Since(start).Nanoseconds())
	atomic.AddInt64(&s.stats.LatencyCount, 1)

	// Send success response
	_, err = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		atomic.AddInt64(&s.stats.ConnsFailed, 1)
		return
	}

	// Bidirectional copy with pooled buffers
	n1, n2 := pool.PooledCopy2Ways(client, target, s.pool)
	atomic.AddInt64(&s.stats.BytesIn, n1)
	atomic.AddInt64(&s.stats.BytesOut, n2)
}

func (s *Server) handleHTTP(client net.Conn, peeked []byte, reader io.Reader) {
	br := bufio.NewReader(reader)
	req, err := http.ReadRequest(br)
	if err != nil {
		atomic.AddInt64(&s.stats.ConnsFailed, 1)
		return
	}

	// Build the target URL
	targetURL := req.URL.String()
	if !req.URL.IsAbs() {
		targetURL = fmt.Sprintf("http://%s%s", req.Host, req.URL.Path)
		if req.URL.RawQuery != "" {
			targetURL += "?" + req.URL.RawQuery
		}
	}

	start := time.Now()

	// Forward the request
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    false,
	}
	defer transport.CloseIdleConnections()

	resp, err := transport.RoundTrip(req)
	if err != nil {
		atomic.AddInt64(&s.stats.ConnsFailed, 1)
		return
	}
	defer resp.Body.Close()

	// Record latency
	atomic.AddInt64(&s.stats.LatencySum, time.Since(start).Nanoseconds())
	atomic.AddInt64(&s.stats.LatencyCount, 1)

	// Write response
	resp.Write(client)

	// Count bytes (approximate)
	atomic.AddInt64(&s.stats.BytesOut, req.ContentLength)
	atomic.AddInt64(&s.stats.BytesIn, resp.ContentLength)
}

// SOCKS5 proxy
type SOCKS5Server struct {
	addr    string
	stats   Stats
	pool    *pool.BufferPool
	tcpOpts *tcpopt.Options
}

func NewSOCKS5Server(addr string) *SOCKS5Server {
	return &SOCKS5Server{
		addr:    addr,
		pool:    pool.NewBufferPool(),
		tcpOpts: tcpopt.AggressiveOptions(),
		stats: Stats{
			StartTime: time.Now().Unix(),
		},
	}
}

func (ss *SOCKS5Server) GetStats() Stats {
	return Stats{
		BytesIn:     atomic.LoadInt64(&ss.stats.BytesIn),
		BytesOut:    atomic.LoadInt64(&ss.stats.BytesOut),
		ConnsActive: atomic.LoadInt32(&ss.stats.ConnsActive),
		ConnsTotal:  atomic.LoadInt64(&ss.stats.ConnsTotal),
		StartTime:   atomic.LoadInt64(&ss.stats.StartTime),
	}
}

func (ss *SOCKS5Server) Start(ctx context.Context) error {
	lc := net.ListenConfig{KeepAlive: 30 * time.Second}
	listener, err := lc.Listen(ctx, "tcp", ss.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", ss.addr, err)
	}
	log.Printf("[SOCKS5] Listening on %s", ss.addr)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				continue
			}
		}

		atomic.AddInt32(&ss.stats.ConnsActive, 1)
		atomic.AddInt64(&ss.stats.ConnsTotal, 1)
		tcpopt.Apply(conn, ss.tcpOpts)
		go ss.handleSOCKS5(conn)
	}
}

func (ss *SOCKS5Server) handleSOCKS5(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[SOCKS5] Recovered from panic: %v", r)
		}
	}()
	defer conn.Close()
	defer atomic.AddInt32(&ss.stats.ConnsActive, -1)

	conn.SetDeadline(time.Now().Add(30 * time.Second))
	buf := make([]byte, 256)

	// Auth handshake
	_, err := io.ReadFull(conn, buf[:2])
	if err != nil {
		return
	}

	nMethods := int(buf[1])
	if nMethods > 0 {
		_, err = io.ReadFull(conn, buf[:nMethods])
		if err != nil {
			return
		}
	}

	// No auth
	conn.Write([]byte{0x05, 0x00})

	// Read request
	_, err = io.ReadFull(conn, buf[:4])
	if err != nil {
		return
	}

	cmd := buf[1]
	atyp := buf[3]

	if cmd != 1 { // Only CONNECT
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return
	}

	var host string
	var port uint16

	switch atyp {
	case 1: // IPv4
		_, err = io.ReadFull(conn, buf[:4])
		if err != nil {
			return
		}
		host = net.IP(buf[:4]).String()
	case 3: // Domain
		_, err = io.ReadFull(conn, buf[:1])
		if err != nil {
			return
		}
		domainLen := int(buf[0])
		_, err = io.ReadFull(conn, buf[:domainLen])
		if err != nil {
			return
		}
		host = string(buf[:domainLen])
	case 4: // IPv6
		_, err = io.ReadFull(conn, buf[:16])
		if err != nil {
			return
		}
		host = net.IP(buf[:16]).String()
	default:
		return
	}

	_, err = io.ReadFull(conn, buf[:2])
	if err != nil {
		return
	}
	port = binary.BigEndian.Uint16(buf[:2])

	targetAddr := fmt.Sprintf("%s:%d", host, port)

	// Connect with optimizations
	target, err := tcpopt.DialWithOpts("tcp", targetAddr, ss.tcpOpts)
	if err != nil {
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return
	}
	defer target.Close()

	// Success response
	localAddr := target.LocalAddr().(*net.TCPAddr)
	resp := make([]byte, 10)
	resp[0] = 0x05
	resp[1] = 0x00
	resp[2] = 0x00
	resp[3] = 0x01
	copy(resp[4:8], localAddr.IP.To4())
	binary.BigEndian.PutUint16(resp[8:10], uint16(localAddr.Port))
	conn.Write(resp)

	conn.SetDeadline(time.Time{})

	// Pooled bidirectional copy
	n1, n2 := pool.PooledCopy2Ways(conn, target, ss.pool)
	atomic.AddInt64(&ss.stats.BytesIn, n1)
	atomic.AddInt64(&ss.stats.BytesOut, n2)
}

// bytesReader implements io.Reader for a byte slice
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// handleSOCKS5FromReader handles SOCKS5 from a reader with peeked data
func (s *Server) handleSOCKS5FromReader(conn net.Conn, reader io.Reader, peeked []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Proxy] Recovered from panic: %v", r)
		}
	}()
	// Read remaining SOCKS5 handshake data
	br := bufio.NewReader(reader)
	
	// Read auth methods (skip version and count, already peeked)
	nMethodsBuf := make([]byte, 1)
	_, err := io.ReadFull(br, nMethodsBuf)
	if err != nil {
		return
	}
	nMethods := int(nMethodsBuf[0])
	
	methods := make([]byte, nMethods)
	_, err = io.ReadFull(br, methods)
	if err != nil {
		return
	}
	
	// No auth
	conn.Write([]byte{0x05, 0x00})
	
	// Read request
	reqHeader := make([]byte, 4)
	_, err = io.ReadFull(br, reqHeader)
	if err != nil {
		return
	}
	
	cmd := reqHeader[1]
	atyp := reqHeader[3]
	
	if cmd != 1 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return
	}
	
	var host string
	var port uint16
	
	switch atyp {
	case 1:
		b := make([]byte, 4)
		io.ReadFull(br, b)
		host = net.IP(b).String()
	case 3:
		b := make([]byte, 1)
		io.ReadFull(br, b)
		l := int(b[0])
		b2 := make([]byte, l)
		io.ReadFull(br, b2)
		host = string(b2)
	case 4:
		b := make([]byte, 16)
		io.ReadFull(br, b)
		host = net.IP(b).String()
	}
	
	pb := make([]byte, 2)
	io.ReadFull(br, pb)
	port = binary.BigEndian.Uint16(pb)
	
	targetAddr := fmt.Sprintf("%s:%d", host, port)
	target, err := tcpopt.DialWithOpts("tcp", targetAddr, s.tcpOpts)
	if err != nil {
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return
	}
	defer target.Close()
	
	localAddr := target.LocalAddr().(*net.TCPAddr)
	resp := make([]byte, 10)
	resp[0] = 0x05
	resp[1] = 0x00
	resp[2] = 0x00
	resp[3] = 0x01
	copy(resp[4:8], localAddr.IP.To4())
	binary.BigEndian.PutUint16(resp[8:10], uint16(localAddr.Port))
	conn.Write(resp)
	
	conn.SetDeadline(time.Time{})
	n1, n2 := pool.PooledCopy2Ways(conn, target, s.pool)
	atomic.AddInt64(&s.stats.BytesIn, n1)
	atomic.AddInt64(&s.stats.BytesOut, n2)
}

// IsSOCKS5Request checks if data looks like a SOCKS5 request
func IsSOCKS5Request(data []byte) bool {
	return len(data) > 0 && data[0] == 5
}

// IsCONNECTRequest checks if data looks like an HTTP CONNECT request
func IsCONNECTRequest(data []byte) bool {
	return len(data) >= 7 && strings.ToUpper(string(data[:7])) == "CONNECT"
}
