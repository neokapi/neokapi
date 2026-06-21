package pluginhost

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/segment"
)

// pluginSegmenterDefaultOrder sorts plugin engines after the built-ins (srx=0,
// uax29=10, sat/llm=20/30) in the selector when a manifest gives no explicit
// order.
const pluginSegmenterDefaultOrder = 100

// RegisterModeCSegmenters walks every Mode-C plugin in host and registers the
// segmentation engines it declares (manifest capabilities.segmenters) into the
// global segment registry. Each becomes a selectable engine carrying the
// parameter schema loaded from the plugin's manifest, dispatched to the plugin's
// Segment RPC over the Mode-C daemon by the generic daemonSegmenter.
//
// No per-plugin code lives here: adding a segmenter plugin requires only its
// manifest entry (+ optional schema file). Registration is idempotent — a name
// already owned by a built-in or an earlier plugin is left untouched, so repeat
// scans (the desktop's plugin watcher) never panic or clobber.
//
// When pool is nil this is a no-op (engines can't be driven without a daemon
// pool). Returns true when at least one new engine was registered, so callers
// can refresh anything derived from the engine set (e.g. the segmentation tool
// schema).
func RegisterModeCSegmenters(host *Host, pool *DaemonPool) bool {
	if host == nil || pool == nil {
		return false
	}
	changed := false
	for _, route := range host.SegmenterRoutes() {
		plugin := route.Plugin
		s := route.Segmenter

		order := s.Order
		if order == 0 {
			order = pluginSegmenterDefaultOrder
		}
		label := s.DisplayName
		if label == "" {
			label = s.Name
		}
		pluginRef := plugin
		engineName := s.Name

		if segment.RegisterIfAbsent(segment.EngineDescriptor{
			Name:        s.Name,
			Label:       label,
			Description: s.Description,
			Order:       order,
			Schema:      loadSegmenterSchema(plugin, s),
			New: func(base segment.BaseConfig, params map[string]any) (segment.Segmenter, error) {
				return newDaemonSegmenter(pool, pluginRef, engineName, base, params), nil
			},
		}) {
			changed = true
		}
	}
	return changed
}

// loadSegmenterSchema loads a segmenter's parameter schema from the JSON file the
// manifest points at, relative to the plugin dir. Returns nil when no schema is
// declared or the file can't be read/parsed — the engine then contributes a
// selector option with no configurable parameters rather than failing wiring.
func loadSegmenterSchema(plugin *Plugin, s manifest.Segmenter) *schema.ComponentSchema {
	if s.Schema == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(plugin.Dir, s.Schema))
	if err != nil {
		return nil
	}
	var cs schema.ComponentSchema
	if err := json.Unmarshal(data, &cs); err != nil {
		return nil
	}
	return &cs
}
