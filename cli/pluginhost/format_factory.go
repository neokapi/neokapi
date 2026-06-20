package pluginhost

import (
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/registry"
)

// RegisterModeCFormats walks every Mode-C plugin in host and registers
// the formats it declares (via manifest capabilities.formats) into reg
// as daemon-backed readers and writers.
//
// A reader is registered when the format declares the "read" capability
// (the daemon will be asked to stream parts back via Process). A writer
// is registered when the format declares "write" (Process is driven in
// read-write mode and the daemon writes the output).
//
// When pool is nil, this is a no-op — daemons can't be spawned without
// a pool, so registering the factories would just produce errors at
// runtime. Callers should wire the App's lazy DaemonPool() before
// calling.
//
// Conflicts (a plugin format with the same name as a built-in or as
// another plugin's format) are silently skipped: the FormatRegistry
// prefers the first registration; we don't overwrite. A future caller
// can use FormatRoute precedence to make this explicit.
func RegisterModeCFormats(host *Host, pool *DaemonPool, reg *registry.FormatRegistry) {
	if host == nil || pool == nil || reg == nil {
		return
	}
	for _, route := range host.formatRoutes() {
		plugin := route.Plugin
		f := route.Format
		formatID := registry.FormatID(f.Name)

		sig := format.FormatSignature{
			MIMETypes:  append([]string(nil), f.MimeTypes...),
			Extensions: append([]string(nil), f.Extensions...),
		}

		// Precedence (AD-007): an installed plugin's format reader/writer
		// overrides a built-in of the same name — installing a plugin for a
		// format is an explicit signal to prefer it (e.g. kapi-pdfium replaces
		// the core hand-rolled PDF reader). Two plugins keep first-registered-
		// wins. Capture the prior source BEFORE re-seeding the info below.
		prior := reg.FormatInfo(formatID)
		overridable := prior == nil || prior.Source == "" || prior.Source == registry.SourceBuiltIn

		// Always seed format metadata (so `kapi formats list` shows it
		// and detection by extension/MIME works) — even when no factory
		// is registered yet.
		reg.RegisterFormatInfo(formatID, registry.FormatInfo{
			Name:        formatID,
			DisplayName: f.DisplayName,
			MimeTypes:   sig.MIMETypes,
			Extensions:  sig.Extensions,
			Source:      plugin.Name(),
			HasReader:   f.HasCapability("read"),
			HasWriter:   f.HasCapability("write"),
			Generative:  f.HasCapability("generative"),
			Interchange: f.HasCapability("interchange"),
		})
		reg.SetFormatSource(formatID, plugin.Name())

		if f.HasCapability("read") && (overridable || !reg.HasReader(formatID)) {
			displayName := f.DisplayName
			pluginRef := plugin
			fmtName := f.Name
			sigCopy := sig
			reg.RegisterReader(formatID, func() format.DataFormatReader {
				// Wrap with the host-side vision tier-3 pass. It is a transparent
				// pass-through unless the format config requests "tier3" and the
				// kapi-vision layout engine is available, so wrapping every plugin
				// reader is free (AD-028: tier 3 applies to any format that can
				// produce a page raster + positioned blocks).
				return newTier3Reader(newDaemonReader(pool, pluginRef, fmtName, sigCopy, displayName))
			}, sig, displayName)
		}
		if f.HasCapability("write") && (overridable || !reg.HasWriter(formatID)) {
			pluginRef := plugin
			fmtName := f.Name
			reg.RegisterWriter(formatID, func() format.DataFormatWriter {
				return newDaemonWriter(pool, pluginRef, fmtName)
			})
		}
	}
}

// formatRoutes returns every FormatRoute the host knows about. Unlike
// the public CommandRoutes / MCPRoutes, format routes don't yet need a
// stable public listing API — but the format-factory code wants one, so
// we expose it here as a package-private helper.
func (h *Host) formatRoutes() []*FormatRoute {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*FormatRoute, 0, len(h.formatDispatch))
	for _, r := range h.formatDispatch {
		out = append(out, r)
	}
	return out
}
