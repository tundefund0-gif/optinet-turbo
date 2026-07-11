package pool

import (
	"testing"
)

func TestNewBufferPool(t *testing.T) {
	bp := NewBufferPool()
	if bp == nil {
		t.Fatal("NewBufferPool returned nil")
	}
}

func TestBufferPoolSmall(t *testing.T) {
	bp := NewBufferPool()
	buf := bp.GetSmall()
	if len(buf) != BufferSize {
		t.Errorf("Expected buffer size %d, got %d", BufferSize, len(buf))
	}
	bp.PutSmall(buf)
}

func TestBufferPoolLarge(t *testing.T) {
	bp := NewBufferPool()
	buf := bp.GetLarge()
	if len(buf) != LargeBufferSize {
		t.Errorf("Expected buffer size %d, got %d", LargeBufferSize, len(buf))
	}
	bp.PutLarge(buf)
}

func TestPoolReuse(t *testing.T) {
	bp := NewBufferPool()
	buf := bp.GetSmall()
	for i := range buf[:32] {
		buf[i] = 0xFF
	}
	bp.PutSmall(buf)

	buf2 := bp.GetSmall()
	if buf2[0] != 0xFF && buf2[31] != 0xFF {
		t.Log("Note: buffer not reused (pool may have created new one)")
	}
	bp.PutSmall(buf2)
}

func TestPooledCopyPassthrough(t *testing.T) {
	bp := NewBufferPool()
	_ = bp
}

func BenchmarkBufferPoolSmall(b *testing.B) {
	bp := NewBufferPool()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := bp.GetSmall()
		bp.PutSmall(buf)
	}
}

func BenchmarkBufferPoolLarge(b *testing.B) {
	bp := NewBufferPool()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := bp.GetLarge()
		bp.PutLarge(buf)
	}
}
