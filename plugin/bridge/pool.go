package bridge

import (
	"fmt"
	"log"
	"sync"
)

// BridgePool manages a pool of JavaBridge instances for concurrent access.
// Each bridge is exclusively leased to one goroutine for the full
// Open→Read→Close lifecycle, eliminating the need for session-level locking.
//
// The pool grows lazily up to maxSize. The first bridge is typically seeded
// during plugin loading (needed for ListFilters discovery). Additional
// bridges start on demand when the idle channel is empty.
type BridgePool struct {
	cfg    BridgeConfig
	logger *log.Logger

	mu      sync.Mutex       // protects created count and closed flag
	idle    chan *JavaBridge // buffered to maxSize
	maxSize int
	created int
	closed  bool
}

// NewBridgePool creates a pool that manages up to maxSize JavaBridge instances.
func NewBridgePool(cfg BridgeConfig, maxSize int, logger *log.Logger) *BridgePool {
	if maxSize < 1 {
		maxSize = 1
	}
	return &BridgePool{
		cfg:     cfg,
		logger:  logger,
		idle:    make(chan *JavaBridge, maxSize),
		maxSize: maxSize,
	}
}

// Seed donates an already-started bridge into the pool (e.g. the discovery
// bridge used for ListFilters). The bridge counts toward the created total.
func (p *BridgePool) Seed(b *JavaBridge) {
	p.mu.Lock()
	p.created++
	p.mu.Unlock()
	p.idle <- b
}

// Acquire returns an idle bridge or starts a new one if the pool has capacity.
// If all bridges are busy and the pool is at capacity, it blocks until one is
// released. Returns an error if the pool has been shut down.
func (p *BridgePool) Acquire() (*JavaBridge, error) {
	// Fast path: grab an idle bridge without locking.
	select {
	case b, ok := <-p.idle:
		if ok {
			return b, nil
		}
		return nil, fmt.Errorf("bridge pool is shut down")
	default:
	}

	// Try to grow the pool.
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("bridge pool is shut down")
	}
	if p.created < p.maxSize {
		p.created++
		p.mu.Unlock()

		b := NewJavaBridge(p.cfg, p.logger)
		if err := b.Start(); err != nil {
			p.mu.Lock()
			p.created--
			p.mu.Unlock()
			return nil, fmt.Errorf("starting new bridge: %w", err)
		}
		return b, nil
	}
	p.mu.Unlock()

	// Pool is full — block until a bridge is returned.
	b, ok := <-p.idle
	if !ok {
		return nil, fmt.Errorf("bridge pool is shut down")
	}
	return b, nil
}

// Release returns a bridge to the pool for reuse by another goroutine.
func (p *BridgePool) Release(b *JavaBridge) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = b.Stop()
		return
	}
	p.mu.Unlock()
	p.idle <- b
}

// Shutdown stops all bridges and closes the pool. Future Acquire calls
// will return an error.
func (p *BridgePool) Shutdown() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	close(p.idle)
	for b := range p.idle {
		_ = b.Stop()
	}
}
