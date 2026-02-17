package bridge

import (
	"fmt"
	"log"
	"sync"
)

// BridgePool manages a process-wide pool of JavaBridge instances, keyed by
// JARPath. Different plugin versions (different JARs) share one global
// concurrency budget: the total number of running JVMs never exceeds maxSize.
//
// Idle bridges are bucketed by JARPath. When a caller requests a bridge for a
// JAR that has no idle instances and the pool is at capacity, the pool evicts
// an idle bridge from a different JAR bucket (stopping it and starting a new
// one for the requested JAR). If no idle bridges exist anywhere, the caller
// blocks until one is released.
type BridgePool struct {
	mu      sync.Mutex
	cond    *sync.Cond
	maxSize int
	active  int // total running bridges (idle + in-use)
	closed  bool
	logger  *log.Logger
	idle    map[string][]*JavaBridge // keyed by JARPath
}

// PoolStats is a snapshot of pool state for debugging and logging.
type PoolStats struct {
	MaxSize   int
	Active    int
	InUse     int
	IdleByJAR map[string]int
}

// NewBridgePool creates a process-wide pool that manages up to maxSize
// JavaBridge instances across all JAR paths.
func NewBridgePool(maxSize int, logger *log.Logger) *BridgePool {
	if maxSize < 1 {
		maxSize = 1
	}
	p := &BridgePool{
		maxSize: maxSize,
		logger:  logger,
		idle:    make(map[string][]*JavaBridge),
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// Seed donates an already-started bridge into the pool (e.g. the discovery
// bridge used for ListFilters). The bridge counts toward the active total.
func (p *BridgePool) Seed(b *JavaBridge) {
	key := b.cfg.JARPath
	p.mu.Lock()
	p.idle[key] = append(p.idle[key], b)
	p.active++
	p.mu.Unlock()
	p.cond.Broadcast()
}

// Acquire returns an idle bridge matching cfg.JARPath, or starts a new one if
// the pool has capacity. If the pool is at capacity but idle bridges exist for
// a different JAR, one is evicted (stopped) to make room. If all bridges are
// in-use with none idle anywhere, the caller blocks until one is released.
func (p *BridgePool) Acquire(cfg BridgeConfig) (*JavaBridge, error) {
	p.mu.Lock()
	for {
		// Step 2: pool closed?
		if p.closed {
			p.mu.Unlock()
			return nil, fmt.Errorf("bridge pool is shut down")
		}

		// Step 3: idle bridge for this JAR?
		key := cfg.JARPath
		if list := p.idle[key]; len(list) > 0 {
			b := list[len(list)-1] // LIFO for cache warmth
			p.idle[key] = list[:len(list)-1]
			if len(p.idle[key]) == 0 {
				delete(p.idle, key)
			}
			p.mu.Unlock()
			return b, nil
		}

		// Step 4: capacity available — create new bridge.
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

		// Step 5: evict an idle bridge from a different JAR bucket.
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

		// Step 6: all bridges in-use, none idle — wait.
		p.cond.Wait()
	}
}

// Release returns a bridge to the pool for reuse. If the pool has been shut
// down, the bridge is stopped instead.
func (p *BridgePool) Release(b *JavaBridge) {
	key := b.cfg.JARPath
	p.mu.Lock()
	if p.closed {
		p.active--
		p.mu.Unlock()
		_ = b.Stop()
		p.cond.Broadcast()
		return
	}
	p.idle[key] = append(p.idle[key], b)
	p.mu.Unlock()
	p.cond.Broadcast()
}

// Shutdown stops all idle bridges and marks the pool as closed. Future Acquire
// calls return an error. In-use bridges are stopped when their holders call
// Release.
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
	byJAR := make(map[string]int, len(p.idle))
	for key, list := range p.idle {
		byJAR[key] = len(list)
		totalIdle += len(list)
	}
	return PoolStats{
		MaxSize:   p.maxSize,
		Active:    p.active,
		InUse:     p.active - totalIdle,
		IdleByJAR: byJAR,
	}
}
