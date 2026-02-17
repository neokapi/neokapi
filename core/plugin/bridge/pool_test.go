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
func newMockBridge(t *testing.T, jarPath string) *JavaBridge {
	t.Helper()
	goStdinR, goStdinW := io.Pipe()
	javaStdoutR, _ := io.Pipe()

	// Close the reader so writes to stdin fail immediately (unblocks Stop).
	goStdinR.Close()

	b := &JavaBridge{
		cfg: BridgeConfig{
			JARPath:        jarPath,
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
	pool := NewBridgePool(0, nil)
	assert.Equal(t, 1, pool.maxSize)
}

func TestPoolSeedAndAcquire(t *testing.T) {
	b := newMockBridge(t, "/path/to/a.jar")
	pool := NewBridgePool(2, nil)
	pool.Seed(b)

	got, err := pool.Acquire(b.cfg)
	require.NoError(t, err)
	assert.Equal(t, b, got)
}

func TestPoolReleaseThenAcquire(t *testing.T) {
	b := newMockBridge(t, "/path/to/a.jar")
	pool := NewBridgePool(1, nil)
	pool.Seed(b)

	got, err := pool.Acquire(b.cfg)
	require.NoError(t, err)

	pool.Release(got)

	got2, err := pool.Acquire(b.cfg)
	require.NoError(t, err)
	assert.Equal(t, b, got2)
}

func TestPoolBlocksWhenFull(t *testing.T) {
	b := newMockBridge(t, "/path/to/a.jar")
	pool := NewBridgePool(1, nil)
	pool.Seed(b)

	// Acquire the only bridge.
	got, err := pool.Acquire(b.cfg)
	require.NoError(t, err)

	acquired := make(chan *JavaBridge, 1)
	go func() {
		b2, _ := pool.Acquire(b.cfg)
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
	b := newMockBridge(t, "/path/to/a.jar")
	pool := NewBridgePool(2, nil)
	pool.Seed(b)

	pool.Shutdown()

	_, err := pool.Acquire(b.cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shut down")
}

func TestPoolShutdownIdempotent(t *testing.T) {
	pool := NewBridgePool(1, nil)
	pool.Shutdown()
	pool.Shutdown() // should not panic
}

func TestPoolReleaseAfterShutdown(t *testing.T) {
	b := newMockBridge(t, "/path/to/a.jar")
	pool := NewBridgePool(1, nil)
	pool.Seed(b)

	got, err := pool.Acquire(b.cfg)
	require.NoError(t, err)

	pool.Shutdown()

	// Release after shutdown should not panic (bridge gets stopped).
	pool.Release(got)
}

func TestPoolConcurrentAcquireRelease(t *testing.T) {
	b1 := newMockBridge(t, "/path/to/a.jar")
	b2 := newMockBridge(t, "/path/to/a.jar")
	pool := NewBridgePool(2, nil)
	pool.Seed(b1)
	pool.Seed(b2)

	cfg := b1.cfg
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			b, err := pool.Acquire(cfg)
			require.NoError(t, err)
			// Simulate some work.
			time.Sleep(time.Millisecond)
			pool.Release(b)
		})
	}
	wg.Wait()
}

func TestPoolMultiJARAcquire(t *testing.T) {
	bA := newMockBridge(t, "/path/to/a.jar")
	bB := newMockBridge(t, "/path/to/b.jar")
	pool := NewBridgePool(4, nil)
	pool.Seed(bA)
	pool.Seed(bB)

	gotA, err := pool.Acquire(bA.cfg)
	require.NoError(t, err)
	assert.Equal(t, "/path/to/a.jar", gotA.cfg.JARPath)

	gotB, err := pool.Acquire(bB.cfg)
	require.NoError(t, err)
	assert.Equal(t, "/path/to/b.jar", gotB.cfg.JARPath)

	pool.Release(gotA)
	pool.Release(gotB)
}

func TestPoolEvictsIdleBridgeForDifferentJAR(t *testing.T) {
	// Fill the pool entirely with JAR-A bridges.
	pool := NewBridgePool(2, nil)
	bA1 := newMockBridge(t, "/path/to/a.jar")
	bA2 := newMockBridge(t, "/path/to/a.jar")
	pool.Seed(bA1)
	pool.Seed(bA2)

	// Acquire one and release it so we have: 1 in-use, 1 idle (both JAR-A).
	inUse, err := pool.Acquire(bA1.cfg)
	require.NoError(t, err)

	// Now request JAR-B. Pool is at capacity (2 active), but there's an idle
	// JAR-A bridge. It should be evicted to make room.
	cfgB := BridgeConfig{
		JARPath:        "/path/to/b.jar",
		CommandTimeout: 5 * time.Second,
		StartupTimeout: 5 * time.Second,
	}

	// This will fail because NewJavaBridge.Start() requires a real JVM,
	// but it demonstrates the eviction path. The idle bA2 gets stopped.
	_, err = pool.Acquire(cfgB)
	// We expect an error because there's no real java binary to start.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "starting new bridge after eviction")

	// The evicted bridge reduced active count, so stats should reflect that.
	stats := pool.Stats()
	assert.Equal(t, 1, stats.Active) // only the in-use JAR-A bridge remains

	pool.Release(inUse)
}

func TestPoolSharedCapacity(t *testing.T) {
	pool := NewBridgePool(3, nil)

	bA := newMockBridge(t, "/path/to/a.jar")
	bB := newMockBridge(t, "/path/to/b.jar")
	bC := newMockBridge(t, "/path/to/c.jar")
	pool.Seed(bA)
	pool.Seed(bB)
	pool.Seed(bC)

	stats := pool.Stats()
	assert.Equal(t, 3, stats.Active)
	assert.Equal(t, 0, stats.InUse)
	assert.Equal(t, 3, stats.MaxSize)

	// Acquire all three.
	gotA, err := pool.Acquire(bA.cfg)
	require.NoError(t, err)
	gotB, err := pool.Acquire(bB.cfg)
	require.NoError(t, err)
	gotC, err := pool.Acquire(bC.cfg)
	require.NoError(t, err)

	stats = pool.Stats()
	assert.Equal(t, 3, stats.Active)
	assert.Equal(t, 3, stats.InUse)

	pool.Release(gotA)
	pool.Release(gotB)
	pool.Release(gotC)
}

func TestPoolBlocksWhenAllActiveNoIdle(t *testing.T) {
	pool := NewBridgePool(1, nil)
	b := newMockBridge(t, "/path/to/a.jar")
	pool.Seed(b)

	// Acquire the only bridge.
	got, err := pool.Acquire(b.cfg)
	require.NoError(t, err)

	// Request a different JAR — no idle bridges at all, should block.
	cfgB := BridgeConfig{JARPath: "/path/to/b.jar", CommandTimeout: 5 * time.Second}
	acquired := make(chan struct{}, 1)
	go func() {
		// This will block since all bridges are in-use.
		_, _ = pool.Acquire(cfgB)
		acquired <- struct{}{}
	}()

	select {
	case <-acquired:
		t.Fatal("should block when all bridges are in-use")
	case <-time.After(50 * time.Millisecond):
		// Expected: blocked.
	}

	// Release makes a bridge idle, which unblocks the waiter.
	// The waiter will try to evict the idle JAR-A bridge and start JAR-B,
	// which will fail (no java), but it unblocks.
	pool.Release(got)

	select {
	case <-acquired:
		// Expected: unblocked (with an error from Start, but unblocked).
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocked Acquire to unblock")
	}
}

func TestPoolStats(t *testing.T) {
	pool := NewBridgePool(4, nil)

	bA1 := newMockBridge(t, "/path/to/a.jar")
	bA2 := newMockBridge(t, "/path/to/a.jar")
	bB := newMockBridge(t, "/path/to/b.jar")
	pool.Seed(bA1)
	pool.Seed(bA2)
	pool.Seed(bB)

	stats := pool.Stats()
	assert.Equal(t, 4, stats.MaxSize)
	assert.Equal(t, 3, stats.Active)
	assert.Equal(t, 0, stats.InUse)
	assert.Equal(t, 2, stats.IdleByJAR["/path/to/a.jar"])
	assert.Equal(t, 1, stats.IdleByJAR["/path/to/b.jar"])

	// Acquire one JAR-A bridge.
	gotA, err := pool.Acquire(bA1.cfg)
	require.NoError(t, err)

	stats = pool.Stats()
	assert.Equal(t, 3, stats.Active)
	assert.Equal(t, 1, stats.InUse)
	assert.Equal(t, 1, stats.IdleByJAR["/path/to/a.jar"])
	assert.Equal(t, 1, stats.IdleByJAR["/path/to/b.jar"])

	pool.Release(gotA)

	stats = pool.Stats()
	assert.Equal(t, 0, stats.InUse)
}
