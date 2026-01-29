// Package loader discovers and loads gokapi plugins from a directory.
// It supports both Go binary plugins (via host.PluginManager) and
// Java bridge plugins (via bridge.JavaBridge) described by *.bridge.json files.
package loader

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/registry"
	"github.com/asgeirf/gokapi/plugin/bridge"
	"github.com/asgeirf/gokapi/plugin/host"
)

// OriginalContentSetter is implemented by writers that need the original
// document content as a skeleton (e.g., bridge format writers).
type OriginalContentSetter interface {
	SetOriginalContent(content []byte)
}

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	Name    string
	Type    string // "binary" or "bridge"
	Source  string
	Formats []string
}

// managedBridge tracks a running Java bridge pool.
type managedBridge struct {
	pool       *bridge.BridgePool
	descriptor *ParsedBridgeDescriptor
	formats    []string
}

// PluginLoader discovers and loads plugins from a directory.
type PluginLoader struct {
	dir     string
	manager *host.PluginManager
	bridges []*managedBridge
	plugins []PluginInfo
	logger  *log.Logger
}

// NewPluginLoader creates a new PluginLoader for the given directory.
func NewPluginLoader(dir string, logger *log.Logger) *PluginLoader {
	return &PluginLoader{
		dir:    dir,
		logger: logger,
	}
}

// LoadAll discovers and loads all plugins from the configured directory.
// Go binary plugins are loaded via host.PluginManager.
// Bridge plugins are loaded from *.bridge.json descriptors.
// If the directory does not exist, this is a no-op.
func (l *PluginLoader) LoadAll(formatReg *registry.FormatRegistry, toolReg *registry.ToolRegistry) error {
	if l.dir == "" {
		return nil
	}

	info, err := os.Stat(l.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking plugin directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("plugin path is not a directory: %s", l.dir)
	}

	// Load Go binary plugins.
	l.manager = host.NewPluginManager(l.logger)
	if err := l.manager.DiscoverAndRegister(l.dir, formatReg); err != nil {
		l.logf("binary plugin discovery: %v", err)
	}

	// Collect binary plugin info.
	for _, detail := range l.manager.PluginDetails() {
		l.plugins = append(l.plugins, PluginInfo{
			Name:    detail.Name,
			Type:    "binary",
			Source:  detail.Source,
			Formats: []string{detail.Name},
		})
	}

	// Load bridge plugins from *.bridge.json descriptors.
	descriptors, err := filepath.Glob(filepath.Join(l.dir, "*.bridge.json"))
	if err != nil {
		l.logf("scanning bridge descriptors: %v", err)
	}

	for _, descPath := range descriptors {
		if err := l.loadBridge(descPath, formatReg); err != nil {
			l.logf("loading bridge %s: %v", descPath, err)
		}
	}

	return nil
}

func (l *PluginLoader) loadBridge(descPath string, formatReg *registry.FormatRegistry) error {
	parsed, err := ParseBridgeDescriptor(descPath, l.dir)
	if err != nil {
		return err
	}

	cfg := bridge.BridgeConfig{
		JavaPath:       parsed.Java,
		JARPath:        parsed.ResolvedJARPath,
		JVMArgs:        parsed.JVMArgs,
		StartupTimeout: parsed.ResolvedStartupTimeout,
		CommandTimeout: parsed.ResolvedCommandTimeout,
	}

	// Start the first bridge for filter discovery, then seed it into the pool.
	b := bridge.NewJavaBridge(cfg, l.logger)
	if err := b.Start(); err != nil {
		return fmt.Errorf("starting bridge %q: %w", parsed.Name, err)
	}

	filters, err := b.ListFilters()
	if err != nil {
		_ = b.Stop()
		return fmt.Errorf("listing filters from bridge %q: %w", parsed.Name, err)
	}

	pool := bridge.NewBridgePool(cfg, runtime.NumCPU(), l.logger)
	pool.Seed(b)

	mb := &managedBridge{
		pool:       pool,
		descriptor: parsed,
	}

	for _, f := range filters.Filters {
		fmtName := parsed.Name + "-" + sanitizeFilterName(f.Name)
		mb.formats = append(mb.formats, fmtName)

		filterClass := f.FilterClass
		bridgePool := pool

		if formatReg != nil {
			formatReg.RegisterReader(fmtName, func() format.DataFormatReader {
				return bridge.NewBridgeFormatReader(bridgePool, filterClass)
			})
			formatReg.RegisterWriter(fmtName, func() format.DataFormatWriter {
				return bridge.NewBridgeFormatWriter(bridgePool, filterClass)
			})
		}

		l.logf("registered bridge format: %s (filter: %s)", fmtName, filterClass)
	}

	l.bridges = append(l.bridges, mb)
	l.plugins = append(l.plugins, PluginInfo{
		Name:    parsed.Name,
		Type:    "bridge",
		Source:  parsed.SourcePath,
		Formats: mb.formats,
	})

	return nil
}

// Plugins returns information about all loaded plugins.
func (l *PluginLoader) Plugins() []PluginInfo {
	return l.plugins
}

// Dir returns the plugin directory path.
func (l *PluginLoader) Dir() string {
	return l.dir
}

// Shutdown stops all plugin processes.
func (l *PluginLoader) Shutdown() {
	if l.manager != nil {
		l.manager.Shutdown()
	}
	for _, mb := range l.bridges {
		mb.pool.Shutdown()
	}
	l.bridges = nil
}

func (l *PluginLoader) logf(format string, args ...interface{}) {
	if l.logger != nil {
		l.logger.Printf(format, args...)
	}
}

// sanitizeFilterName converts an Okapi filter display name to a CLI-friendly slug.
func sanitizeFilterName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	return name
}
