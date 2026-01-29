package bridge

import (
	"bufio"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockBridge creates a JavaBridge backed by pipes (no real JVM).
// The stdin reader side is closed so that Stop() doesn't block on writes.
func newMockBridge(t *testing.T) *JavaBridge {
	t.Helper()
	goStdinR, goStdinW := io.Pipe()
	javaStdoutR, _ := io.Pipe()

	// Close the reader so writes to stdin fail immediately (unblocks Stop).
	goStdinR.Close()

	b := &JavaBridge{
		cfg: BridgeConfig{
			CommandTimeout: 5 * time.Second,
			StartupTimeout: 5 * time.Second,
		},
		stdin:   goStdinW,
		scanner: bufio.NewScanner(javaStdoutR),
		logger:  log.New(io.Discard, "", 0),
		running: true,
	}
	b.scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	return b
}

func TestNewBridgePoolMinSize(t *testing.T) {
	cfg := BridgeConfig{}
	pool := NewBridgePool(cfg, 0, nil)
	assert.Equal(t, 1, pool.maxSize)
}

func TestPoolSeedAndAcquire(t *testing.T) {
	b := newMockBridge(t)
	pool := NewBridgePool(b.cfg, 2, nil)
	pool.Seed(b)

	got, err := pool.Acquire()
	require.NoError(t, err)
	assert.Equal(t, b, got)
}

func TestPoolReleaseThenAcquire(t *testing.T) {
	b := newMockBridge(t)
	pool := NewBridgePool(b.cfg, 1, nil)
	pool.Seed(b)

	got, err := pool.Acquire()
	require.NoError(t, err)

	pool.Release(got)

	got2, err := pool.Acquire()
	require.NoError(t, err)
	assert.Equal(t, b, got2)
}

func TestPoolBlocksWhenFull(t *testing.T) {
	b := newMockBridge(t)
	pool := NewBridgePool(b.cfg, 1, nil)
	pool.Seed(b)

	// Acquire the only bridge.
	got, err := pool.Acquire()
	require.NoError(t, err)

	acquired := make(chan *JavaBridge, 1)
	go func() {
		b2, _ := pool.Acquire()
		acquired <- b2
	}()

	// Should not acquire immediately since pool is exhausted.
	select {
	case <-acquired:
		t.Fatal("should not have acquired a bridge while pool is full")
	case <-time.After(50 * time.Millisecond):
		// Expected: blocked.
	}

	// Release the bridge — the blocked goroutine should unblock.
	pool.Release(got)

	select {
	case b2 := <-acquired:
		assert.Equal(t, b, b2)
		pool.Release(b2)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Acquire after Release")
	}
}

func TestPoolShutdown(t *testing.T) {
	b := newMockBridge(t)
	pool := NewBridgePool(b.cfg, 2, nil)
	pool.Seed(b)

	pool.Shutdown()

	_, err := pool.Acquire()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shut down")
}

func TestPoolShutdownIdempotent(t *testing.T) {
	pool := NewBridgePool(BridgeConfig{}, 1, nil)
	pool.Shutdown()
	pool.Shutdown() // should not panic
}

func TestPoolReleaseAfterShutdown(t *testing.T) {
	b := newMockBridge(t)
	pool := NewBridgePool(b.cfg, 1, nil)
	pool.Seed(b)

	got, err := pool.Acquire()
	require.NoError(t, err)

	pool.Shutdown()

	// Release after shutdown should not panic (bridge gets stopped).
	pool.Release(got)
}

func TestPoolConcurrentAcquireRelease(t *testing.T) {
	b1 := newMockBridge(t)
	b2 := newMockBridge(t)
	pool := NewBridgePool(b1.cfg, 2, nil)
	pool.Seed(b1)
	pool.Seed(b2)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := pool.Acquire()
			require.NoError(t, err)
			// Simulate some work.
			time.Sleep(time.Millisecond)
			pool.Release(b)
		}()
	}
	wg.Wait()
}
