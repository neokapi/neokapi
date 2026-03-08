package bridge

import (
	"fmt"
	"log"
	"sync"
)

// BridgePool manages a process-wide pool of JavaBridge instances, keyed by
// their process configuration (command + args). Different plugin versions share
// one global concurrency budget: the total number of running subprocesses never
// exceeds maxSize.
//
// The pool supports two access modes:
//
//   - Exclusive (Acquire/Release): one goroutine owns the bridge for stateful
//     Open→Read→Close or Write cycles. No other goroutine can use the bridge.
//
//   - Shared (AcquireShared/ReleaseShared): multiple goroutines share a single
//     bridge for concurrent RoundTrip streams. gRPC connections support concurrent
//     streams, and each RoundTrip creates its own filter instance in Java, so
//     there's no shared mutable state. This mode uses one JVM for N files.
type BridgePool struct {
	mu      sync.Mutex
	cond    *sync.Cond
	maxSize int
	active  int // total running bridges (idle + in-use + shared)
	closed  bool
	logger  *log.Logger
	idle    map[string][]*JavaBridge // keyed by PoolKey
	shared  map[string]*sharedEntry // keyed by PoolKey — one shared bridge per config
}

// sharedEntry tracks a bridge with active concurrent sessions.
type sharedEntry struct {
	bridge   *JavaBridge
	sessions int
}

// PoolStats is a snapshot of pool state for debugging and logging.
type PoolStats struct {
	MaxSize        int
	Active         int
	InUse          int
	SharedSessions int
	IdleByKey      map[string]int
}

// NewBridgePool creates a process-wide pool that manages up to maxSize
// JavaBridge instances across all configurations.
func NewBridgePool(maxSize int, logger *log.Logger) *BridgePool {
	if maxSize < 1 {
		maxSize = 1
	}
	p := &BridgePool{
		maxSize: maxSize,
		logger:  logger,
		idle:    make(map[string][]*JavaBridge),
		shared:  make(map[string]*sharedEntry),
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// Seed donates an already-started bridge into the pool (e.g. the discovery
// bridge used for ListFilters). The bridge counts toward the active total.
func (p *BridgePool) Seed(b *JavaBridge) {
	key := b.cfg.PoolKey()
	p.mu.Lock()
	p.idle[key] = append(p.idle[key], b)
	p.active++
	p.mu.Unlock()
	p.cond.Broadcast()
}

// Acquire returns an idle bridge matching cfg's PoolKey, or starts a new one if
// the pool has capacity. If the pool is at capacity but idle bridges exist for
// a different configuration, one is evicted (stopped) to make room. If all
// bridges are in-use with none idle anywhere, the caller blocks until one is
// released.
func (p *BridgePool) Acquire(cfg BridgeConfig) (*JavaBridge, error) {
	p.mu.Lock()
	for {
		if p.closed {
			p.mu.Unlock()
			return nil, fmt.Errorf("bridge pool is shut down")
		}

		key := cfg.PoolKey()
		if list := p.idle[key]; len(list) > 0 {
			b := list[len(list)-1] // LIFO for cache warmth
			p.idle[key] = list[:len(list)-1]
			if len(p.idle[key]) == 0 {
				delete(p.idle, key)
			}
			p.mu.Unlock()
			return b, nil
		}

		// Capacity available — create new bridge.
		if p.active < p.maxSize {
			p.active++
			p.mu.Unlock()

			b := NewJavaBridge(cfg, p.logger)
			if err := b.Start(); err != nil {
				p.mu.Lock()
				p.active--
				p.mu.Unlock()
				p.cond.Broadcast()
				return nil, fmt.Errorf("starting new bridge: %w", err)
			}
			return b, nil
		}

		// Evict an idle bridge from a different bucket.
		var victim *JavaBridge
		for otherKey, list := range p.idle {
			if len(list) > 0 {
				victim = list[len(list)-1]
				p.idle[otherKey] = list[:len(list)-1]
				if len(p.idle[otherKey]) == 0 {
					delete(p.idle, otherKey)
				}
				break
			}
		}
		if victim != nil {
			// active stays the same: we replace one bridge with another.
			p.mu.Unlock()

			_ = victim.Stop()

			b := NewJavaBridge(cfg, p.logger)
			if err := b.Start(); err != nil {
				p.mu.Lock()
				p.active--
				p.mu.Unlock()
				p.cond.Broadcast()
				return nil, fmt.Errorf("starting new bridge after eviction: %w", err)
			}
			return b, nil
		}

		// All bridges in-use, none idle — wait.
		p.cond.Wait()
	}
}

// Release returns a bridge to the pool for reuse. If the pool has been shut
// down or the bridge is unhealthy, the bridge is stopped and discarded.
func (p *BridgePool) Release(b *JavaBridge) {
	key := b.cfg.PoolKey()
	p.mu.Lock()
	if p.closed {
		p.active--
		p.mu.Unlock()
		_ = b.Stop()
		p.cond.Broadcast()
		return
	}

	// Health check: discard bridges with broken gRPC connections or
	// stale JVM state rather than polluting the pool.
	if !b.IsHealthy() {
		p.active--
		p.mu.Unlock()
		if p.logger != nil {
			p.logger.Printf("[bridge-pool] discarding unhealthy bridge (key=%s)", key)
		}
		_ = b.Stop()
		p.cond.Broadcast()
		return
	}

	p.idle[key] = append(p.idle[key], b)
	p.mu.Unlock()
	p.cond.Broadcast()
}

// AcquireShared returns a bridge for concurrent RoundTrip use. Multiple callers
// can share the same bridge — each gets its own gRPC stream. The bridge stays
// active until the last session calls ReleaseShared.
//
// If no shared bridge exists for this config, one is taken from the idle pool
// or created (following the same capacity/eviction logic as Acquire).
func (p *BridgePool) AcquireShared(cfg BridgeConfig) (*JavaBridge, error) {
	p.mu.Lock()
	for {
		if p.closed {
			p.mu.Unlock()
			return nil, fmt.Errorf("bridge pool is shut down")
		}

		key := cfg.PoolKey()

		// Reuse existing shared bridge for this config.
		if se, ok := p.shared[key]; ok {
			se.sessions++
			b := se.bridge
			p.mu.Unlock()
			return b, nil
		}

		// Try to promote an idle bridge to shared.
		if list := p.idle[key]; len(list) > 0 {
			b := list[len(list)-1]
			p.idle[key] = list[:len(list)-1]
			if len(p.idle[key]) == 0 {
				delete(p.idle, key)
			}
			p.shared[key] = &sharedEntry{bridge: b, sessions: 1}
			p.mu.Unlock()
			return b, nil
		}

		// Capacity available — create new bridge.
		if p.active < p.maxSize {
			p.active++
			p.mu.Unlock()

			b := NewJavaBridge(cfg, p.logger)
			if err := b.Start(); err != nil {
				p.mu.Lock()
				p.active--
				p.mu.Unlock()
				p.cond.Broadcast()
				return nil, fmt.Errorf("starting shared bridge: %w", err)
			}

			p.mu.Lock()
			p.shared[key] = &sharedEntry{bridge: b, sessions: 1}
			p.mu.Unlock()
			return b, nil
		}

		// Evict an idle bridge from a different bucket.
		var victim *JavaBridge
		for otherKey, list := range p.idle {
			if len(list) > 0 {
				victim = list[len(list)-1]
				p.idle[otherKey] = list[:len(list)-1]
				if len(p.idle[otherKey]) == 0 {
					delete(p.idle, otherKey)
				}
				break
			}
		}
		if victim != nil {
			p.mu.Unlock()
			_ = victim.Stop()

			b := NewJavaBridge(cfg, p.logger)
			if err := b.Start(); err != nil {
				p.mu.Lock()
				p.active--
				p.mu.Unlock()
				p.cond.Broadcast()
				return nil, fmt.Errorf("starting shared bridge after eviction: %w", err)
			}

			p.mu.Lock()
			p.shared[key] = &sharedEntry{bridge: b, sessions: 1}
			p.mu.Unlock()
			return b, nil
		}

		// All bridges in-use, none idle — wait.
		p.cond.Wait()
	}
}

// ReleaseShared decrements the session count for a shared bridge. When the last
// session releases, the bridge moves to the idle pool for reuse.
func (p *BridgePool) ReleaseShared(b *JavaBridge) {
	key := b.cfg.PoolKey()
	p.mu.Lock()

	se, ok := p.shared[key]
	if !ok {
		// Not tracked as shared — fall back to normal release.
		p.mu.Unlock()
		p.Release(b)
		return
	}

	se.sessions--
	if se.sessions > 0 {
		// Other sessions still active — nothing to do.
		p.mu.Unlock()
		return
	}

	// Last session done — remove from shared and return to idle.
	delete(p.shared, key)

	if p.closed {
		p.active--
		p.mu.Unlock()
		_ = b.Stop()
		p.cond.Broadcast()
		return
	}

	if !b.IsHealthy() {
		p.active--
		p.mu.Unlock()
		if p.logger != nil {
			p.logger.Printf("[bridge-pool] discarding unhealthy shared bridge (key=%s)", key)
		}
		_ = b.Stop()
		p.cond.Broadcast()
		return
	}

	p.idle[key] = append(p.idle[key], b)
	p.mu.Unlock()
	p.cond.Broadcast()
}

// Warmup eagerly starts a bridge for the given config and places it in the
// idle pool. This amortizes JVM startup before files arrive. Returns nil if
// the pool is already at capacity or a bridge for this config is already available.
func (p *BridgePool) Warmup(cfg BridgeConfig) error {
	p.mu.Lock()
	key := cfg.PoolKey()

	// Already have one ready — skip.
	if len(p.idle[key]) > 0 || p.shared[key] != nil {
		p.mu.Unlock()
		return nil
	}

	if p.active >= p.maxSize || p.closed {
		p.mu.Unlock()
		return nil
	}

	p.active++
	p.mu.Unlock()

	b := NewJavaBridge(cfg, p.logger)
	if err := b.Start(); err != nil {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
		p.cond.Broadcast()
		return fmt.Errorf("warmup bridge: %w", err)
	}

	p.mu.Lock()
	p.idle[key] = append(p.idle[key], b)
	p.mu.Unlock()
	p.cond.Broadcast()
	return nil
}

// Shutdown stops all idle bridges and marks the pool as closed. Future Acquire
// calls return an error. In-use bridges are stopped when their holders call
// Release. Shared bridges with active sessions are stopped when the last
// session calls ReleaseShared.
func (p *BridgePool) Shutdown() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true

	// Collect all idle bridges to stop outside the lock.
	var toStop []*JavaBridge
	for key, list := range p.idle {
		toStop = append(toStop, list...)
		p.active -= len(list)
		delete(p.idle, key)
	}
	p.mu.Unlock()

	// Wake all waiters so they see closed=true and return errors.
	p.cond.Broadcast()

	for _, b := range toStop {
		_ = b.Stop()
	}
}

// Stats returns a snapshot of pool state for debugging and logging.
func (p *BridgePool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	totalIdle := 0
	byKey := make(map[string]int, len(p.idle))
	for key, list := range p.idle {
		byKey[key] = len(list)
		totalIdle += len(list)
	}

	totalShared := 0
	for _, se := range p.shared {
		totalShared += se.sessions
	}

	return PoolStats{
		MaxSize:        p.maxSize,
		Active:         p.active,
		InUse:          p.active - totalIdle,
		SharedSessions: totalShared,
		IdleByKey:      byKey,
	}
}
