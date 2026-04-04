package bridge

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// BridgeRegistry manages one JVM per Okapi version with semaphore-based
// concurrency control. In daemon mode, JVMs persist across process
// invocations and are discovered via address files on disk.
type BridgeRegistry struct {
	mu          sync.Mutex
	bridges     map[string]*managedBridge
	global      chan struct{} // global concurrency semaphore
	maxPerJVM   int
	logger      *log.Logger
	closed      bool
	daemon      bool
	idleTimeout time.Duration
	cacheDir    string // ~/.cache/neokapi/bridge/
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
	MaxTotal    int
	MaxPerJVM   int
	BridgeCount int
	GlobalInUse int
	BridgeByKey map[string]BridgeStats
	DaemonMode  bool
}

// BridgeStats tracks per-bridge concurrency usage.
type BridgeStats struct {
	InUse    int
	Capacity int
}

// NewBridgeRegistry creates a registry that manages one JVM per configuration.
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
	cacheDir, _ := os.UserCacheDir()
	return &BridgeRegistry{
		bridges:   make(map[string]*managedBridge),
		global:    global,
		maxPerJVM: maxPerJVM,
		logger:    logger,
		cacheDir:  filepath.Join(cacheDir, "kapi", "bridge"),
	}
}

// SetDaemonMode enables daemon mode. In daemon mode, the registry checks for
// existing JVMs via address files and starts new JVMs with --idle-timeout.
func (r *BridgeRegistry) SetDaemonMode(enabled bool, idleTimeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.daemon = enabled
	r.idleTimeout = idleTimeout
}

// Acquire returns a bridge for the given configuration, acquiring both global
// and per-JVM semaphore slots. The JVM is started lazily on first use.
// In daemon mode, tries to connect to an existing JVM first.
func (r *BridgeRegistry) Acquire(cfg BridgeConfig) (*JavaBridge, func(), error) {
	// Acquire global semaphore slot.
	<-r.global

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
func (r *BridgeRegistry) getOrCreate(cfg BridgeConfig) (*managedBridge, error) {
	key := cfg.PoolKey()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("bridge registry is shut down")
	}

	if mb, ok := r.bridges[key]; ok {
		r.mu.Unlock()
		<-mb.ready
		if mb.err != nil {
			return nil, mb.err
		}
		return mb, nil
	}

	// First caller for this key.
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

	// In daemon mode, try to connect to an existing JVM first.
	if r.daemon {
		if addr, err := r.readAddrFile(key); err == nil && addr != "" {
			daemonCfg := BridgeConfig{
				Address:        addr,
				CommandTimeout: cfg.withDefaults().CommandTimeout,
				StartupTimeout: cfg.withDefaults().StartupTimeout,
			}
			b := NewJavaBridge(daemonCfg, r.logger)
			if err := b.Start(); err == nil {
				r.logf("connected to daemon bridge at %s", addr)
				mb.bridge = b
				close(mb.ready)
				return mb, nil
			}
			// Stale address file — remove it and start fresh.
			r.logf("stale daemon at %s, starting new", addr)
			r.removeAddrFile(key)
		}
	}

	// Start a new JVM subprocess.
	startCfg := cfg
	if r.daemon && r.idleTimeout > 0 {
		// Append idle timeout flag so the JVM stays alive after we disconnect.
		startCfg.Args = append(append([]string{}, cfg.Args...), "--idle-timeout",
			strconv.Itoa(int(r.idleTimeout.Seconds())))
	}

	b := NewJavaBridge(startCfg, r.logger)
	if err := b.Start(); err != nil {
		mb.err = fmt.Errorf("starting bridge: %w", err)
		close(mb.ready)
		r.mu.Lock()
		delete(r.bridges, key)
		r.mu.Unlock()
		return nil, mb.err
	}

	// In daemon mode, write address file for future connections.
	if r.daemon && b.Address() != "" {
		r.writeAddrFile(key, b.Address())
	}

	mb.bridge = b
	close(mb.ready)
	return mb, nil
}

// Warmup eagerly starts a JVM for the given configuration.
func (r *BridgeRegistry) Warmup(cfg BridgeConfig) error {
	_, err := r.getOrCreate(cfg)
	return err
}

// Shutdown stops all JVMs and marks the registry as closed.
// In daemon mode, JVMs with idle timeout are left running (they'll self-terminate).
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
		if r.daemon {
			// In daemon mode, don't send Shutdown — let idle timeout handle it.
			// Just close the gRPC connection.
			b.Disconnect()
		} else {
			_ = b.Stop()
		}
	}
}

// Stats returns a snapshot of registry state.
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
		DaemonMode:  r.daemon,
	}
}

// --- Address file management ---

func (r *BridgeRegistry) addrFilePath(key string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))[:16]
	return filepath.Join(r.cacheDir, hash, "addr")
}

func (r *BridgeRegistry) readAddrFile(key string) (string, error) {
	data, err := os.ReadFile(r.addrFilePath(key))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r *BridgeRegistry) writeAddrFile(key, addr string) {
	path := r.addrFilePath(key)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.logf("failed to create addr dir %s: %v", dir, err)
		return
	}
	// Atomic write via temp file + rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(addr), 0o644); err != nil {
		r.logf("failed to write addr file: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		r.logf("failed to rename addr file: %v", err)
		os.Remove(tmp)
	}
}

func (r *BridgeRegistry) removeAddrFile(key string) {
	path := r.addrFilePath(key)
	os.Remove(path)
	os.Remove(filepath.Dir(path)) // remove empty dir
}

func (r *BridgeRegistry) logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Printf("[bridge-registry] "+format, args...)
	}
}
