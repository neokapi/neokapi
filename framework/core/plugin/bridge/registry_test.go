package bridge

import (
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockBridge creates a JavaBridge with no real JVM process (for registry tests).
func newMockBridge(t *testing.T, args ...string) *JavaBridge {
	t.Helper()
	return &JavaBridge{
		cfg: BridgeConfig{
			Command:        "java",
			Args:           args,
			CommandTimeout: 5 * time.Second,
			StartupTimeout: 5 * time.Second,
		},
		logger:  log.New(io.Discard, "", 0),
		running: true,
	}
}

func TestNewBridgeRegistryMinValues(t *testing.T) {
	r := NewBridgeRegistry(0, 0, nil)
	assert.Equal(t, 1, cap(r.global))
	assert.Equal(t, 1, r.maxPerJVM)
}

func TestRegistryAcquireAndRelease(t *testing.T) {
	r := NewBridgeRegistry(4, 2, log.New(io.Discard, "", 0))
	defer r.Shutdown()

	b := newMockBridge(t, "-jar", "/path/to/a.jar")
	cfg := b.cfg

	// Inject a bridge directly.
	r.mu.Lock()
	key := cfg.PoolKey()
	sem := make(chan struct{}, 2)
	sem <- struct{}{}
	sem <- struct{}{}
	r.bridges[key] = &managedBridge{bridge: b, sem: sem, cfg: cfg}
	r.mu.Unlock()

	got, release, err := r.Acquire(cfg)
	require.NoError(t, err)
	assert.Equal(t, b, got)

	// Release and re-acquire.
	release()

	got2, release2, err := r.Acquire(cfg)
	require.NoError(t, err)
	assert.Equal(t, b, got2)
	release2()
}

func TestRegistryShutdown(t *testing.T) {
	r := NewBridgeRegistry(1, 1, nil)
	r.Shutdown()

	_, _, err := r.Acquire(BridgeConfig{Command: "java"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shut down")
}

func TestRegistryShutdownIdempotent(t *testing.T) {
	r := NewBridgeRegistry(1, 1, nil)
	r.Shutdown()
	r.Shutdown() // should not panic
}

func TestRegistryConcurrentAcquireRelease(t *testing.T) {
	r := NewBridgeRegistry(4, 4, log.New(io.Discard, "", 0))
	defer r.Shutdown()

	b := newMockBridge(t, "-jar", "/path/to/a.jar")
	cfg := b.cfg

	r.mu.Lock()
	key := cfg.PoolKey()
	sem := make(chan struct{}, 4)
	for range 4 {
		sem <- struct{}{}
	}
	r.bridges[key] = &managedBridge{bridge: b, sem: sem, cfg: cfg}
	r.mu.Unlock()

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			got, release, err := r.Acquire(cfg)
			require.NoError(t, err)
			assert.Equal(t, b, got)
			time.Sleep(time.Millisecond)
			release()
		})
	}
	wg.Wait()
}

func TestRegistryStats(t *testing.T) {
	r := NewBridgeRegistry(4, 2, log.New(io.Discard, "", 0))
	defer r.Shutdown()

	b := newMockBridge(t, "-jar", "/path/to/a.jar")
	cfg := b.cfg

	r.mu.Lock()
	key := cfg.PoolKey()
	sem := make(chan struct{}, 2)
	sem <- struct{}{}
	sem <- struct{}{}
	r.bridges[key] = &managedBridge{bridge: b, sem: sem, cfg: cfg}
	r.mu.Unlock()

	stats := r.Stats()
	assert.Equal(t, 4, stats.MaxTotal)
	assert.Equal(t, 2, stats.MaxPerJVM)
	assert.Equal(t, 1, stats.BridgeCount)
	assert.Equal(t, 0, stats.GlobalInUse)
	assert.Equal(t, 0, stats.BridgeByKey[key].InUse)

	got, release, err := r.Acquire(cfg)
	require.NoError(t, err)
	assert.Equal(t, b, got)

	stats = r.Stats()
	assert.Equal(t, 1, stats.GlobalInUse)
	assert.Equal(t, 1, stats.BridgeByKey[key].InUse)

	release()

	stats = r.Stats()
	assert.Equal(t, 0, stats.GlobalInUse)
	assert.Equal(t, 0, stats.BridgeByKey[key].InUse)
}
