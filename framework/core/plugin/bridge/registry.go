package bridge

import (
	"fmt"
	"log"
	"sync"
)

// BridgeRegistry manages one JVM per Okapi version with semaphore-based
// concurrency control. Unlike BridgePool which maintained multiple JVM
// instances, BridgeRegistry starts a single JVM per configuration (version)
// and controls concurrent access via semaphores.
type BridgeRegistry struct {
	mu        sync.Mutex
	bridges   map[string]*managedBridge
	global    chan struct{} // global concurrency semaphore
	maxPerJVM int
	logger    *log.Logger
	closed    bool
}

// managedBridge tracks a single JVM bridge instance with its concurrency semaphore.
type managedBridge struct {
	bridge *JavaBridge
	sem    chan struct{} // per-JVM concurrency semaphore
	cfg    BridgeConfig
	ready  chan struct{} // closed when bridge is started (or failed)
	err    error         // non-nil if Start() failed
}

// RegistryStats is a snapshot of registry state for debugging and logging.
type RegistryStats struct {
	MaxTotal     int
	MaxPerJVM    int
	BridgeCount  int
	GlobalInUse  int
	BridgeByKey  map[string]BridgeStats
}

// BridgeStats tracks per-bridge concurrency usage.
type BridgeStats struct {
	InUse    int
	Capacity int
}

// NewBridgeRegistry creates a registry that manages one JVM per configuration.
// maxTotal limits the total concurrent operations across all JVMs.
// maxPerJVM limits concurrent operations per individual JVM.
func NewBridgeRegistry(maxTotal, maxPerJVM int, logger *log.Logger) *BridgeRegistry {
	if maxTotal < 1 {
		maxTotal = 1
	}
	if maxPerJVM < 1 {
		maxPerJVM = 1
	}
	global := make(chan struct{}, maxTotal)
	for range maxTotal {
		global <- struct{}{}
	}
	return &BridgeRegistry{
		bridges:   make(map[string]*managedBridge),
		global:    global,
		maxPerJVM: maxPerJVM,
		logger:    logger,
	}
}

// Acquire returns a bridge for the given configuration, acquiring both global
// and per-JVM semaphore slots. The JVM is started lazily on first use.
// The returned release function must be called when done.
func (r *BridgeRegistry) Acquire(cfg BridgeConfig) (*JavaBridge, func(), error) {
	// Acquire global semaphore slot.
	select {
	case <-r.global:
	default:
		// Block until a slot is available.
		<-r.global
	}

	mb, err := r.getOrCreate(cfg)
	if err != nil {
		r.global <- struct{}{} // return global slot
		return nil, nil, err
	}

	if mb.bridge == nil {
		r.global <- struct{}{}
		return nil, nil, fmt.Errorf("bridge not initialized for key %s", cfg.PoolKey())
	}

	// Acquire per-JVM semaphore slot.
	<-mb.sem

	released := false
	release := func() {
		if released {
			return
		}
		released = true
		mb.sem <- struct{}{}
		r.global <- struct{}{}
	}
	return mb.bridge, release, nil
}

// getOrCreate returns an existing managed bridge or creates a new one.
// Concurrent callers for the same key wait until the bridge is ready.
func (r *BridgeRegistry) getOrCreate(cfg BridgeConfig) (*managedBridge, error) {
	key := cfg.PoolKey()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, fmt.Errorf("bridge registry is shut down")
	}

	if mb, ok := r.bridges[key]; ok {
		r.mu.Unlock()
		// Wait for bridge to be ready (handles concurrent first-access race).
		<-mb.ready
		if mb.err != nil {
			return nil, mb.err
		}
		return mb, nil
	}

	// First caller for this key — create and start the bridge.
	mb := &managedBridge{
		cfg:   cfg,
		ready: make(chan struct{}),
	}
	sem := make(chan struct{}, r.maxPerJVM)
	for range r.maxPerJVM {
		sem <- struct{}{}
	}
	mb.sem = sem
	r.bridges[key] = mb
	r.mu.Unlock()

	b := NewJavaBridge(cfg, r.logger)
	if err := b.Start(); err != nil {
		mb.err = fmt.Errorf("starting bridge: %w", err)
		close(mb.ready)
		r.mu.Lock()
		delete(r.bridges, key)
		r.mu.Unlock()
		return nil, mb.err
	}

	mb.bridge = b
	close(mb.ready) // Signal all waiters that the bridge is ready.
	return mb, nil
}

// Warmup eagerly starts a JVM for the given configuration.
func (r *BridgeRegistry) Warmup(cfg BridgeConfig) error {
	_, err := r.getOrCreate(cfg)
	return err
}

// Shutdown stops all JVMs and marks the registry as closed.
func (r *BridgeRegistry) Shutdown() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true

	var toStop []*JavaBridge
	for _, mb := range r.bridges {
		if mb.bridge != nil {
			toStop = append(toStop, mb.bridge)
		}
	}
	r.bridges = make(map[string]*managedBridge)
	r.mu.Unlock()

	for _, b := range toStop {
		_ = b.Stop()
	}
}

// Stats returns a snapshot of registry state for debugging and logging.
func (r *BridgeRegistry) Stats() RegistryStats {
	r.mu.Lock()
	defer r.mu.Unlock()

	byKey := make(map[string]BridgeStats, len(r.bridges))
	for key, mb := range r.bridges {
		byKey[key] = BridgeStats{
			InUse:    r.maxPerJVM - len(mb.sem),
			Capacity: r.maxPerJVM,
		}
	}

	return RegistryStats{
		MaxTotal:    cap(r.global),
		MaxPerJVM:   r.maxPerJVM,
		BridgeCount: len(r.bridges),
		GlobalInUse: cap(r.global) - len(r.global),
		BridgeByKey: byKey,
	}
}
