package tcpopt

import (
	"net"
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts == nil {
		t.Fatal("DefaultOptions returned nil")
	}
	if !opts.NoDelay {
		t.Error("Expected NoDelay=true by default")
	}
	if !opts.KeepAlive {
		t.Error("Expected KeepAlive=true by default")
	}
	if opts.ReadBuffer <= 0 {
		t.Error("Expected positive ReadBuffer")
	}
	if opts.WriteBuffer <= 0 {
		t.Error("Expected positive WriteBuffer")
	}
}

func TestAggressiveOptions(t *testing.T) {
	opts := AggressiveOptions()
	if opts == nil {
		t.Fatal("AggressiveOptions returned nil")
	}
	if opts.ReadBuffer < DefaultOptions().ReadBuffer {
		t.Log("Aggressive read buffer is smaller than default")
	}
	if opts.KeepAlivePeriod >= DefaultOptions().KeepAlivePeriod {
		t.Log("Aggressive keepalive should be shorter or equal")
	}
}

func TestApplyNonTCP(t *testing.T) {
	opts := DefaultOptions()
	// Non-TCP connection should not error
	err := Apply(&dummyConn{}, opts)
	if err != nil {
		t.Errorf("Apply to non-TCP conn: %v", err)
	}
}

func TestDialWithOpts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test")
	}
	
	opts := DefaultOptions()
	conn, err := DialWithOpts("tcp", "8.8.8.8:443", opts)
	if err != nil {
		t.Fatalf("DialWithOpts failed: %v", err)
	}
	defer conn.Close()
	
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Fatal("Connection is not TCP")
	}
	
	err = tcpConn.SetNoDelay(true)
	if err != nil {
		t.Logf("Could not set NoDelay: %v", err)
	}
}

type dummyConn struct{}

func (d *dummyConn) Read(b []byte) (n int, err error)  { return len(b), nil }
func (d *dummyConn) Write(b []byte) (n int, err error) { return len(b), nil }
func (d *dummyConn) Close() error                      { return nil }
func (d *dummyConn) LocalAddr() net.Addr               { return &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 8080} }
func (d *dummyConn) RemoteAddr() net.Addr              { return &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 9090} }
func (d *dummyConn) SetDeadline(t time.Time) error      { return nil }
func (d *dummyConn) SetReadDeadline(t time.Time) error  { return nil }
func (d *dummyConn) SetWriteDeadline(t time.Time) error { return nil }

func BenchmarkDialWithOpts(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping network benchmark")
	}
	
	opts := DefaultOptions()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := DialWithOpts("tcp", "8.8.8.8:443", opts)
		if err == nil {
			conn.Close()
		}
	}
}
