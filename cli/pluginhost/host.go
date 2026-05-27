// Package pluginhost is the host-side runtime for kapi's unified plugin
// model (#438). It discovers plugins on disk, builds dispatch tables
// from their manifests, and provides Mode A/B/C entry points.
package pluginhost

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// systemSourceOrder is the Source.Order of OS-managed install roots
// (e.g. /opt/homebrew/share/kapi/plugins). Plugins discovered there are owned
// by the OS package manager and must not be removed by kapi.
const systemSourceOrder = 3

// Source identifies which discovery root a plugin came from. Lower
// numeric values win on conflict (KAPI_PLUGINS_DIR > XDG > system).
type Source struct {
	Order int    // 1 = $KAPI_PLUGINS_DIR, 2 = XDG, 3 = system
	Label string // human-readable, e.g. "$KAPI_PLUGINS_DIR" or "/opt/homebrew/share/kapi/plugins"
	Path  string // absolute filesystem path
}

// Plugin is one discovered plugin: its manifest, install dir, and source.
type Plugin struct {
	// Dir is the absolute path to the plugin's install directory (the
	// dir containing manifest.json).
	Dir string

	// Source identifies which discovery root surfaced this plugin.
	Source Source

	// Manifest is the parsed manifest.json contents.
	Manifest *manifest.Manifest

	// BinaryPath is the absolute path to the plugin's executable
	// (Dir + "/" + Manifest.Binary).
	BinaryPath string
}

// Name returns the plugin's declared name.
func (p *Plugin) Name() string { return p.Manifest.Plugin }

// Version returns the plugin's declared version.
func (p *Plugin) Version() string { return p.Manifest.Version }

// Host is the in-memory state of all discovered plugins for one kapi
// process. It is built once at startup and consumed by command
// dispatch, MCP, format-detection, and recipe schema validation.
type Host struct {
	mu      sync.RWMutex
	plugins []*Plugin

	// Dispatch tables, built from plugins on construction.
	commandDispatch map[string]*CommandRoute      // command name → owning plugin + manifest entry
	mcpDispatch     map[string]*MCPRoute          // MCP tool name → owning plugin + manifest entry
	formatDispatch  map[string]*FormatRoute       // format name → owning plugin + manifest entry
	schemaExt       []SchemaExtensionRegistration // recipe schema extensions surfaced from manifests
	contributions   []*ContributionRoute          // contributions to built-in commands
}

// ContributionRoute names a command contribution and the plugin that owns it.
// Unlike CommandRoute (one plugin owns the whole command), multiple plugins may
// contribute to the same built-in command, so contributions are kept as a slice.
type ContributionRoute struct {
	Plugin       *Plugin
	Contribution manifest.CommandContribution
}

// CommandRoute names a command capability and the plugin it dispatches to.
type CommandRoute struct {
	Plugin  *Plugin
	Command manifest.Command
}

// MCPRoute names an MCP tool capability and the plugin it dispatches to.
type MCPRoute struct {
	Plugin *Plugin
	Tool   manifest.MCPTool
}

// FormatRoute names a format capability and the plugin it dispatches to.
type FormatRoute struct {
	Plugin *Plugin
	Format manifest.Format
}

// SchemaExtensionRegistration pairs a discovered manifest schema_extension
// entry with the plugin that owns it.
type SchemaExtensionRegistration struct {
	Plugin    *Plugin
	Extension manifest.SchemaExtension
}

// NewHost builds a Host from a slice of discovered plugins. Conflicts
// (two plugins claiming the same command/mcp-tool/format name) cause
// the conflicting capability to be omitted from dispatch tables; an
// optional collector receives a description of every conflict.
func NewHost(plugins []*Plugin, conflicts func(msg string)) *Host {
	if conflicts == nil {
		conflicts = func(string) {}
	}
	h := &Host{
		plugins:         plugins,
		commandDispatch: map[string]*CommandRoute{},
		mcpDispatch:     map[string]*MCPRoute{},
		formatDispatch:  map[string]*FormatRoute{},
	}

	// Sort plugins by source precedence (lower = higher priority), then
	// by name for determinism.
	sortedPlugins := append([]*Plugin(nil), plugins...)
	sort.SliceStable(sortedPlugins, func(i, j int) bool {
		if sortedPlugins[i].Source.Order != sortedPlugins[j].Source.Order {
			return sortedPlugins[i].Source.Order < sortedPlugins[j].Source.Order
		}
		return sortedPlugins[i].Name() < sortedPlugins[j].Name()
	})

	// Within a single name, only the highest-precedence plugin survives.
	seenName := map[string]*Plugin{}
	for _, p := range sortedPlugins {
		if existing, ok := seenName[p.Name()]; ok {
			conflicts(fmt.Sprintf(
				"plugin %q declared in both %s and %s — using %s; remove the other to silence this warning",
				p.Name(), existing.Source.Label, p.Source.Label, existing.Source.Label,
			))
			continue
		}
		seenName[p.Name()] = p
	}

	dedup := make([]*Plugin, 0, len(seenName))
	for _, p := range sortedPlugins {
		if seenName[p.Name()] == p {
			dedup = append(dedup, p)
		}
	}
	h.plugins = dedup

	for _, p := range dedup {
		for _, c := range p.Manifest.Capabilities.Commands {
			if existing, ok := h.commandDispatch[c.Name]; ok {
				conflicts(fmt.Sprintf("command %q is provided by plugins %q and %q — neither will dispatch until one is removed", c.Name, existing.Plugin.Name(), p.Name()))
				delete(h.commandDispatch, c.Name)
				continue
			}
			h.commandDispatch[c.Name] = &CommandRoute{Plugin: p, Command: c}
		}
		for _, t := range p.Manifest.Capabilities.MCPTools {
			if existing, ok := h.mcpDispatch[t.Name]; ok {
				conflicts(fmt.Sprintf("MCP tool %q is provided by plugins %q and %q — neither will dispatch until one is removed", t.Name, existing.Plugin.Name(), p.Name()))
				delete(h.mcpDispatch, t.Name)
				continue
			}
			h.mcpDispatch[t.Name] = &MCPRoute{Plugin: p, Tool: t}
		}
		for _, f := range p.Manifest.Capabilities.Formats {
			if existing, ok := h.formatDispatch[f.Name]; ok {
				conflicts(fmt.Sprintf("format %q is provided by plugins %q and %q — neither will dispatch until one is removed", f.Name, existing.Plugin.Name(), p.Name()))
				delete(h.formatDispatch, f.Name)
				continue
			}
			h.formatDispatch[f.Name] = &FormatRoute{Plugin: p, Format: f}
		}
		for _, ext := range p.Manifest.Capabilities.SchemaExtensions {
			h.schemaExt = append(h.schemaExt, SchemaExtensionRegistration{Plugin: p, Extension: ext})
		}
		for _, cc := range p.Manifest.Capabilities.CommandContributions {
			h.contributions = append(h.contributions, &ContributionRoute{Plugin: p, Contribution: cc})
		}
	}

	return h
}

// Plugins returns the deduplicated set of plugins this Host dispatches
// to, in source-precedence + alphabetical order.
func (h *Host) Plugins() []*Plugin {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Plugin, len(h.plugins))
	copy(out, h.plugins)
	return out
}

// Plugin returns the plugin with the given name, or nil if not found.
func (h *Host) Plugin(name string) *Plugin {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, p := range h.plugins {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// Remove uninstalls the named plugin, deleting it from the exact directory it
// was discovered in (Plugin.Dir) and dropping it from the host's dispatch
// tables. This is the single uninstall entry point for every front-end (CLI,
// desktop): callers pass only the plugin name — the plugin system owns where
// each plugin lives, so install, discovery, and removal can never disagree on
// the directory.
//
// It refuses to remove OS-managed (system) installs, which belong to the OS
// package manager, and reports an error when the plugin is not installed.
func (h *Host) Remove(name string) error {
	h.mu.Lock()
	var target *Plugin
	for _, p := range h.plugins {
		if p.Name() == name {
			target = p
			break
		}
	}
	if target == nil {
		h.mu.Unlock()
		return fmt.Errorf("plugin %q is not installed", name)
	}
	if target.Source.Order >= systemSourceOrder {
		label := target.Source.Label
		h.mu.Unlock()
		return fmt.Errorf("plugin %q is a system install (%s) and must be removed via the OS package manager", name, label)
	}
	dir := target.Dir
	// Drop from the in-memory tables so the host stays consistent without a
	// full re-discovery; routing for the removed plugin stops immediately.
	h.dropPluginLocked(target)
	h.mu.Unlock()

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	return nil
}

// dropPluginLocked removes p from the plugins slice and every dispatch table.
// Callers must hold h.mu.
func (h *Host) dropPluginLocked(p *Plugin) {
	kept := h.plugins[:0]
	for _, q := range h.plugins {
		if q != p {
			kept = append(kept, q)
		}
	}
	h.plugins = kept
	for k, r := range h.commandDispatch {
		if r.Plugin == p {
			delete(h.commandDispatch, k)
		}
	}
	for k, r := range h.mcpDispatch {
		if r.Plugin == p {
			delete(h.mcpDispatch, k)
		}
	}
	for k, r := range h.formatDispatch {
		if r.Plugin == p {
			delete(h.formatDispatch, k)
		}
	}
	keptExt := h.schemaExt[:0]
	for _, e := range h.schemaExt {
		if e.Plugin != p {
			keptExt = append(keptExt, e)
		}
	}
	h.schemaExt = keptExt
	keptContrib := h.contributions[:0]
	for _, c := range h.contributions {
		if c.Plugin != p {
			keptContrib = append(keptContrib, c)
		}
	}
	h.contributions = keptContrib
}

// CommandRoute returns the dispatch entry for a top-level command, or
// nil when the command is not provided by any plugin.
func (h *Host) CommandRoute(name string) *CommandRoute {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.commandDispatch[name]
}

// MCPRoute returns the dispatch entry for an MCP tool.
func (h *Host) MCPRoute(name string) *MCPRoute {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.mcpDispatch[name]
}

// FormatRoute returns the dispatch entry for a format.
func (h *Host) FormatRoute(name string) *FormatRoute {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.formatDispatch[name]
}

// CommandRoutes returns all command dispatch entries. The slice is
// sorted by command name for determinism.
func (h *Host) CommandRoutes() []*CommandRoute {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*CommandRoute, 0, len(h.commandDispatch))
	for _, r := range h.commandDispatch {
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Command.Name < out[j].Command.Name
	})
	return out
}

// ContributionRoutes returns all command-contribution entries, sorted by the
// target command name then the owning plugin for deterministic wiring order.
func (h *Host) ContributionRoutes() []*ContributionRoute {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*ContributionRoute, len(h.contributions))
	copy(out, h.contributions)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Contribution.Command != out[j].Contribution.Command {
			return out[i].Contribution.Command < out[j].Contribution.Command
		}
		return out[i].Plugin.Name() < out[j].Plugin.Name()
	})
	return out
}

// MCPRoutes returns all MCP tool dispatch entries, sorted by name.
func (h *Host) MCPRoutes() []*MCPRoute {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*MCPRoute, 0, len(h.mcpDispatch))
	for _, r := range h.mcpDispatch {
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Tool.Name < out[j].Tool.Name
	})
	return out
}

// SchemaExtensions returns all schema_extension registrations surfaced
// by discovered plugins.
func (h *Host) SchemaExtensions() []SchemaExtensionRegistration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]SchemaExtensionRegistration, len(h.schemaExt))
	copy(out, h.schemaExt)
	return out
}
