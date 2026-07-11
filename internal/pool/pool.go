package pool

import (
	"io"
	"net"
	"sync"
)

const (
	// BufferSize is the optimal buffer size for network I/O
	BufferSize = 32 * 1024 // 32KB
	// LargeBufferSize for bigger transfers
	LargeBufferSize = 64 * 1024 // 64KB
)

// BufferPool provides pooled byte buffers for zero-allocation I/O
type BufferPool struct {
	small sync.Pool
	large sync.Pool
}

// NewBufferPool creates a new buffer pool
func NewBufferPool() *BufferPool {
	return &BufferPool{
		small: sync.Pool{
			New: func() interface{} {
				b := make([]byte, BufferSize)
				return &b
			},
		},
		large: sync.Pool{
			New: func() interface{} {
				b := make([]byte, LargeBufferSize)
				return &b
			},
		},
	}
}

// GetSmall returns a small buffer from the pool
func (bp *BufferPool) GetSmall() []byte {
	ptr := bp.small.Get().(*[]byte)
	return *ptr
}

// PutSmall returns a small buffer to the pool
func (bp *BufferPool) PutSmall(b []byte) {
	bp.small.Put(&b)
}

// GetLarge returns a large buffer from the pool
func (bp *BufferPool) GetLarge() []byte {
	ptr := bp.large.Get().(*[]byte)
	return *ptr
}

// PutLarge returns a large buffer to the pool
func (bp *BufferPool) PutLarge(b []byte) {
	bp.large.Put(&b)
}

// PooledCopy copies data between connections using pooled buffers
func PooledCopy(dst net.Conn, src net.Conn, pool *BufferPool) (written int64, err error) {
	buf := pool.GetSmall()
	defer pool.PutSmall(buf)
	return io.CopyBuffer(dst, src, buf)
}

// PooledCopy2Ways performs bidirectional copy with pooled buffers
func PooledCopy2Ways(conn1, conn2 net.Conn, pool *BufferPool) (int64, int64) {
	var wg sync.WaitGroup
	wg.Add(2)

	var n1, n2 int64
	go func() {
		defer wg.Done()
		n1, _ = PooledCopy(conn2, conn1, pool)
	}()
	go func() {
		defer wg.Done()
		n2, _ = PooledCopy(conn1, conn2, pool)
	}()
	wg.Wait()
	return n1, n2
}
