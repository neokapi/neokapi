// Package loader discovers and loads gokapi plugins from a directory.
// It supports both Go binary plugins (via host.PluginManager) and
// Java bridge plugins (via bridge.JavaBridge) described by *.bridge.json files.
//
// The directory layout uses versioned subdirectories:
//
//	{dir}/{packName}/{version}/  — contains bridge descriptors, JARs, or binaries
package loader

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/plugin/bridge"
	"github.com/gokapi/gokapi/core/plugin/host"
	pluginreg "github.com/gokapi/gokapi/core/plugin/registry"
	"github.com/gokapi/gokapi/core/preset"
	"github.com/gokapi/gokapi/core/registry"
)

// OriginalContentSetter is implemented by writers that need the original
// document content as a skeleton (e.g., bridge format writers).
type OriginalContentSetter interface {
	SetOriginalContent(content []byte)
}

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	Name    string
	Version string
	Type    string // "binary" or "bridge"
	Source  string
	Formats []string
}

// managedBridge tracks a loaded Java bridge descriptor.
type managedBridge struct {
	cfg        bridge.BridgeConfig
	descriptor *ParsedBridgeDescriptor
	version    string
	formats    []string
}

// PluginLoader discovers and loads plugins from a directory.
type PluginLoader struct {
	dir     string
	manager *host.PluginManager
	pool    *bridge.BridgePool // single shared pool for all bridge plugins
	bridges []*managedBridge
	plugins []PluginInfo
	schemas *SchemaRegistry        // filter parameter schemas
	presets *preset.PresetRegistry  // format and framework presets
	logger  *log.Logger
}

// NewPluginLoader creates a new PluginLoader for the given directory.
func NewPluginLoader(dir string, logger *log.Logger) *PluginLoader {
	return &PluginLoader{
		dir:     dir,
		schemas: NewSchemaRegistry(),
		presets: preset.NewPresetRegistry(),
		logger:  logger,
	}
}

// LoadAll discovers and loads all plugins from the configured directory.
// It scans for versioned subdirectories ({dir}/{name}/{version}/) and loads
// bridge descriptors and binary plugins from each version directory.
// For each plugin name, both versioned format names (e.g., "okapi-html@1.46.0")
// and bare aliases (pointing to the latest version) are registered.
// If the directory does not exist, this is a no-op.
//
// The toolReg parameter is accepted for future use: tool plugin registration
// is ready but pending bridge-level tool support (steps are not yet exposed
// via the bridge protocol).
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

	l.manager = host.NewPluginManager(l.logger)

	// Load versioned plugins from {dir}/{name}/{version}/ structure.
	all, err := pluginreg.ListAllInstalled(l.dir)
	if err != nil {
		l.logf("scanning versioned plugins: %v", err)
		return nil
	}

	// Track versioned format names per base format name for bare-name aliasing.
	// Key: base format name (e.g., "okapi-html"), Value: list of {version, formatName}.
	type versionedFormat struct {
		version string
		name    string // e.g. "okapi-html@1.46.0"
	}
	bareNameCandidates := make(map[string][]versionedFormat)

	for name, versions := range all {
		// Sort versions so we process them in order.
		sort.Slice(versions, func(i, j int) bool {
			return pluginreg.CompareSemver(versions[i].Version, versions[j].Version) < 0
		})

		for _, iv := range versions {
			vDir := iv.Dir
			switch iv.InstallType {
			case "bridge":
				// Look for *.bridge.json descriptors in the version directory.
				descriptors, err := filepath.Glob(filepath.Join(vDir, "*.bridge.json"))
				if err != nil {
					l.logf("scanning bridge descriptors in %s: %v", vDir, err)
					continue
				}
				for _, descPath := range descriptors {
					formats, err := l.loadBridge(descPath, vDir, iv.Version, formatReg)
					if err != nil {
						l.logf("loading bridge %s: %v", descPath, err)
						continue
					}
					for _, fmtName := range formats {
						// Extract base name from versioned name.
						baseName := fmtName
						if idx := strings.LastIndex(fmtName, "@"); idx > 0 {
							baseName = fmtName[:idx]
						}
						bareNameCandidates[baseName] = append(bareNameCandidates[baseName], versionedFormat{
							version: iv.Version,
							name:    fmtName,
						})
					}
				}

			case "binary":
				// Look for gokapi-plugin-* binaries in the version directory.
				pattern := filepath.Join(vDir, "gokapi-plugin-*")
				binaries, err := filepath.Glob(pattern)
				if err != nil {
					l.logf("scanning binaries in %s: %v", vDir, err)
					continue
				}
				for _, binPath := range binaries {
					if err := l.manager.DiscoverAndRegister(vDir, formatReg); err != nil {
						l.logf("loading binary plugin %s: %v", binPath, err)
					}
				}
				l.plugins = append(l.plugins, PluginInfo{
					Name:    name,
					Version: iv.Version,
					Type:    "binary",
					Source:  vDir,
				})
			}
		}
	}

	// Register bare-name aliases pointing to the latest version.
	if formatReg != nil {
		for baseName, candidates := range bareNameCandidates {
			// Find the latest version.
			best := candidates[0]
			for _, c := range candidates[1:] {
				if pluginreg.CompareSemver(c.version, best.version) > 0 {
					best = c
				}
			}
			// Only register if no bare name is already registered.
			if !formatReg.HasReader(baseName) {
				if formatReg.HasReader(best.name) {
					versionedName := best.name
					formatReg.RegisterReader(baseName, func() format.DataFormatReader {
						r, _ := formatReg.NewReader(versionedName)
						return r
					})
					// Propagate source from the versioned format.
					if info := formatReg.FormatInfo(versionedName); info != nil {
						formatReg.SetFormatSource(baseName, info.Source)
					}
				}
			}
			if !formatReg.HasWriter(baseName) {
				if formatReg.HasWriter(best.name) {
					versionedName := best.name
					formatReg.RegisterWriter(baseName, func() format.DataFormatWriter {
						w, _ := formatReg.NewWriter(versionedName)
						return w
					})
				}
			}
		}
	}

	// Extract bridge configuration presets from loaded schemas.
	l.schemas.ExtractPresets(l.presets)

	// Scan version directories for presets.yaml files.
	for _, versions := range all {
		for _, iv := range versions {
			presetsPath := filepath.Join(iv.Dir, "presets.yaml")
			if _, err := os.Stat(presetsPath); err == nil {
				if err := LoadPresetsFromFile(presetsPath, l.presets, iv.Dir); err != nil {
					l.logf("loading presets from %s: %v", presetsPath, err)
				}
			}
		}
	}

	return nil
}

func (l *PluginLoader) loadBridge(descPath, versionDir, version string, formatReg *registry.FormatRegistry) ([]string, error) {
	parsed, err := ParseBridgeDescriptor(descPath, versionDir)
	if err != nil {
		return nil, err
	}

	cfg := bridge.BridgeConfig{
		JavaPath:       parsed.Java,
		JARPath:        parsed.ResolvedJARPath,
		JVMArgs:        parsed.JVMArgs,
		StartupTimeout: parsed.ResolvedStartupTimeout,
		CommandTimeout: parsed.ResolvedCommandTimeout,
	}

	// Load schemas from the schemas/ subdirectory (NO Java needed).
	schemasDir := filepath.Join(versionDir, "schemas")
	if err := l.schemas.LoadFromDirectory(schemasDir); err != nil {
		l.logf("loading schemas from %s: %v", schemasDir, err)
		// Continue - schemas are optional
	} else if l.schemas.Count() > 0 {
		l.logf("loaded %d filter schemas from %s", l.schemas.Count(), schemasDir)
	}

	// Lazily create the shared pool on first bridge load.
	if l.pool == nil {
		l.pool = bridge.NewBridgePool(runtime.NumCPU(), l.logger)
	}

	// Start the first bridge for filter discovery, then seed it into the shared pool.
	b := bridge.NewJavaBridge(cfg, l.logger)
	if err := b.Start(); err != nil {
		return nil, fmt.Errorf("starting bridge %q: %w", parsed.Name, err)
	}

	filters, err := b.ListFilters()
	if err != nil {
		_ = b.Stop()
		return nil, fmt.Errorf("listing filters from bridge %q: %w", parsed.Name, err)
	}

	l.pool.Seed(b)

	mb := &managedBridge{
		cfg:        cfg,
		descriptor: parsed,
		version:    version,
	}

	sharedPool := l.pool
	bridgeCfg := cfg

	var formats []string
	for _, f := range filters.Filters {
		baseFmtName := parsed.Name + "-" + sanitizeFilterName(f.Name)
		versionedName := baseFmtName + "@" + version
		mb.formats = append(mb.formats, versionedName)
		formats = append(formats, versionedName)

		filterClass := f.FilterClass

		if formatReg != nil {
			formatReg.RegisterReader(versionedName, func() format.DataFormatReader {
				return bridge.NewBridgeFormatReader(sharedPool, bridgeCfg, filterClass)
			})
			formatReg.RegisterWriter(versionedName, func() format.DataFormatWriter {
				return bridge.NewBridgeFormatWriter(sharedPool, bridgeCfg, filterClass)
			})
			formatReg.SetFormatSource(versionedName, parsed.Name)
		}

		l.logf("registered bridge format: %s (filter: %s)", versionedName, filterClass)
	}

	l.bridges = append(l.bridges, mb)
	l.plugins = append(l.plugins, PluginInfo{
		Name:    parsed.Name,
		Version: version,
		Type:    "bridge",
		Source:  parsed.SourcePath,
		Formats: mb.formats,
	})

	return formats, nil
}

// Plugins returns information about all loaded plugins.
func (l *PluginLoader) Plugins() []PluginInfo {
	return l.plugins
}

// Dir returns the plugin directory path.
func (l *PluginLoader) Dir() string {
	return l.dir
}

// Schemas returns the schema registry for filter parameters.
func (l *PluginLoader) Schemas() *SchemaRegistry {
	return l.schemas
}

// Presets returns the preset registry.
func (l *PluginLoader) Presets() *preset.PresetRegistry {
	return l.presets
}

// Shutdown stops all plugin processes.
func (l *PluginLoader) Shutdown() {
	if l.manager != nil {
		l.manager.Shutdown()
	}
	if l.pool != nil {
		l.pool.Shutdown()
		l.pool = nil
	}
	l.bridges = nil
}

func (l *PluginLoader) logf(format string, args ...any) {
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
