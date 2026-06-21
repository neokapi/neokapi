package pluginhost

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/registry"
)

// Runtime is the single place a long- or short-lived front-end (the kapi CLI,
// the desktop app) brings manifest plugins online. It owns the live *Host and
// the Mode-C DaemonPool, and centralizes the discover → build → wire sequence
// (dispatch tables, recipe schema extensions, source-connector dispatchers, and
// daemon-backed formats) so that neither the CLI nor the desktop reimplements
// it. Watch additionally lets a long-lived app react to plugins installed or
// removed by another process (e.g. `kapi plugins install` run in a terminal).
//
// Every method is safe for concurrent use.
type Runtime struct {
	opts                DiscoverOptions
	formatReg           *registry.FormatRegistry
	onWarn              func(string)
	registerConnectors  bool
	useCache            bool
	poolLogger          func(format string, args ...any)
	onSegmentersChanged func()

	mu   sync.RWMutex
	host *Host
	sig  string // signature of the plugin set the current host was built from

	poolMu sync.Mutex
	pool   *DaemonPool
}

// RuntimeOptions configures a Runtime.
type RuntimeOptions struct {
	// Discover controls which roots are scanned for plugins.
	Discover DiscoverOptions

	// FormatReg, when non-nil, receives daemon-backed readers/writers for
	// every Mode-C plugin format on each (re)scan. Nil disables format wiring.
	FormatReg *registry.FormatRegistry

	// OnWarn receives non-fatal warnings (discovery, conflicts, schema). Optional.
	OnWarn func(string)

	// RegisterConnectors enables registering a generic source-connector
	// dispatcher for Mode-C plugins that declare source_connectors. The CLI
	// sets this; front-ends that don't source through plugin connectors leave
	// it off so repeated rescans don't touch the global connector registry.
	RegisterConnectors bool

	// UseCache enables the on-disk plugins cache fast path on Rescan (CLI
	// startup). Leave off for front-ends that must always reflect on-disk
	// truth, such as a desktop app reacting to live installs.
	UseCache bool

	// PoolLogger is the logger handed to the lazily-built DaemonPool. Optional.
	PoolLogger func(format string, args ...any)

	// OnSegmentersChanged is invoked (when set) after a (re)scan registers one
	// or more plugin-provided segmentation engines into the global segment
	// registry. Front-ends use it to refresh anything derived from the engine
	// set — notably the composed segmentation tool schema, so plugin engines
	// appear in the selector. Optional.
	OnSegmentersChanged func()
}

// NewRuntime constructs a Runtime. It performs no discovery until Rescan.
func NewRuntime(o RuntimeOptions) *Runtime {
	onWarn := o.OnWarn
	if onWarn == nil {
		onWarn = func(string) {}
	}
	return &Runtime{
		opts:                o.Discover,
		formatReg:           o.FormatReg,
		onWarn:              onWarn,
		registerConnectors:  o.RegisterConnectors,
		useCache:            o.UseCache,
		poolLogger:          o.PoolLogger,
		onSegmentersChanged: o.OnSegmentersChanged,
	}
}

// Rescan rediscovers plugins, rebuilds the host, and wires schema extensions,
// (optionally) source connectors, and Mode-C formats. Safe to call repeatedly;
// the wiring steps are idempotent. Returns the new host.
func (r *Runtime) Rescan() *Host {
	return r.build(r.discover())
}

// Host returns the current host, or nil before the first Rescan.
func (r *Runtime) Host() *Host {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.host
}

// DaemonPool returns the lazily-constructed Mode-C daemon pool. The pool object
// is created on first call; daemon processes spawn only on first format use.
func (r *Runtime) DaemonPool() *DaemonPool {
	r.poolMu.Lock()
	defer r.poolMu.Unlock()
	if r.pool == nil {
		r.pool = NewDaemonPool(DaemonPoolOptions{Logger: r.poolLogger})
	}
	return r.pool
}

// Shutdown tears down the daemon pool, stopping any running Mode-C daemons.
func (r *Runtime) Shutdown() {
	r.poolMu.Lock()
	pool := r.pool
	r.pool = nil
	r.poolMu.Unlock()
	if pool != nil {
		pool.Shutdown()
	}
}

// Watch polls the discovery roots every interval and, whenever the discovered
// plugin set changes (an install or removal by this or another process),
// rebuilds the host and invokes onChange with the new host. It returns when ctx
// is cancelled. Discovery here always reads from disk (never the cache) so
// external changes are seen promptly.
func (r *Runtime) Watch(ctx context.Context, interval time.Duration, onChange func(*Host)) {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			plugins := Discover(r.opts)
			sig := pluginSignature(plugins)
			r.mu.RLock()
			unchanged := sig == r.sig
			r.mu.RUnlock()
			if unchanged {
				continue
			}
			host := r.build(plugins)
			if onChange != nil {
				onChange(host)
			}
		}
	}
}

// discover returns the current plugin set, honoring the cache fast path when
// UseCache is set (and refreshing the cache on a miss).
func (r *Runtime) discover() []*Plugin {
	if !r.useCache {
		return Discover(r.opts)
	}
	if cache, err := LoadCache(CacheLocation()); err == nil && IsFresh(cache, r.opts) {
		return PluginsFromCache(cache)
	}
	plugins := Discover(r.opts)
	// Best-effort cache write; an unwritable cache dir must not break startup.
	_ = SaveCache(CacheLocation(), BuildCache(r.opts, plugins))
	return plugins
}

// build constructs a host from plugins, wires it, and stores it as current.
func (r *Runtime) build(plugins []*Plugin) *Host {
	host := NewHost(plugins, r.onWarn)
	r.wire(host)
	sig := pluginSignature(plugins)
	r.mu.Lock()
	r.host = host
	r.sig = sig
	r.mu.Unlock()
	return host
}

// wire performs the post-NewHost registration sequence shared by every
// front-end: recipe schema extensions, optional source-connector dispatchers,
// daemon-backed Mode-C formats, and plugin-provided segmentation engines.
func (r *Runtime) wire(host *Host) {
	RegisterSchemaExtensions(host, r.onWarn)

	if r.registerConnectors {
		for _, p := range host.Plugins() {
			if !p.Manifest.IsModeC() {
				continue
			}
			if len(p.Manifest.Capabilities.SourceConnectors) == 0 {
				continue
			}
			RegisterSourceConnectorDispatcher(
				NewGenericSourceConnectorDispatcher(p.Name()),
				SourceConnectorOpsClaimed...,
			)
		}
	}

	if r.formatReg != nil {
		RegisterModeCFormats(host, r.DaemonPool(), r.formatReg)
	}

	// Plugin-provided segmentation engines register into the global segment
	// registry (independent of formatReg). Only touch the daemon pool when a
	// plugin actually declares a segmenter.
	if len(host.SegmenterRoutes()) > 0 {
		if RegisterModeCSegmenters(host, r.DaemonPool()) && r.onSegmentersChanged != nil {
			r.onSegmentersChanged()
		}
	}
}

// pluginSignature returns a stable fingerprint of a plugin set — each plugin's
// install dir and version — so Watch can detect installs, removals, and version
// changes without rebuilding on every poll.
func pluginSignature(plugins []*Plugin) string {
	parts := make([]string, 0, len(plugins))
	for _, p := range plugins {
		ver := ""
		if p.Manifest != nil {
			ver = p.Manifest.Version
		}
		parts = append(parts, p.Dir+"@"+ver)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}
