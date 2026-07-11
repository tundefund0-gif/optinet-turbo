package tcpopt

import (
	"net"
	"time"
)

// Options holds TCP tuning parameters
type Options struct {
	NoDelay        bool
	KeepAlive      bool
	KeepAlivePeriod time.Duration
	ReadBuffer     int
	WriteBuffer    int
	Deadline       time.Duration
	Linger         int
}

// DefaultOptions returns default optimized TCP options
func DefaultOptions() *Options {
	return &Options{
		NoDelay:         true,
		KeepAlive:       true,
		KeepAlivePeriod: 30 * time.Second,
		ReadBuffer:      256 * 1024,  // 256KB
		WriteBuffer:     256 * 1024,  // 256KB
		Deadline:        60 * time.Second,
		Linger:          0,
	}
}

// AggressiveOptions for maximum performance (gaming)
func AggressiveOptions() *Options {
	return &Options{
		NoDelay:         true,
		KeepAlive:       true,
		KeepAlivePeriod: 15 * time.Second,
		ReadBuffer:      512 * 1024,  // 512KB
		WriteBuffer:     512 * 1024,  // 512KB
		Deadline:        30 * time.Second,
		Linger:          0,
	}
}

// Apply applies TCP tuning options to a connection
func Apply(conn net.Conn, opts *Options) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}

	var err error

	if opts.NoDelay {
		if e := tcpConn.SetNoDelay(true); e != nil {
			err = e
		}
	}

	if opts.KeepAlive {
		if e := tcpConn.SetKeepAlive(true); e != nil {
			err = e
		}
		if opts.KeepAlivePeriod > 0 {
			if e := tcpConn.SetKeepAlivePeriod(opts.KeepAlivePeriod); e != nil {
				err = e
			}
		}
	}

	if opts.ReadBuffer > 0 {
		if e := tcpConn.SetReadBuffer(opts.ReadBuffer); e != nil {
			err = e
		}
	}

	if opts.WriteBuffer > 0 {
		if e := tcpConn.SetWriteBuffer(opts.WriteBuffer); e != nil {
			err = e
		}
	}

	if opts.Deadline > 0 {
		if e := tcpConn.SetDeadline(time.Now().Add(opts.Deadline)); e != nil {
			err = e
		}
	}

	if opts.Linger >= 0 {
		if e := tcpConn.SetLinger(opts.Linger); e != nil {
			err = e
		}
	}

	return err
}

// DialWithOpts dials with optimized TCP settings
func DialWithOpts(network, addr string, opts *Options) (net.Conn, error) {
	d := net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: opts.KeepAlivePeriod,
	}

	conn, err := d.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	if err := Apply(conn, opts); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}
