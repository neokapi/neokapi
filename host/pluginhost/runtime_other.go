//go:build !js

package pluginhost

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
