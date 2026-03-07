// Package loader discovers and loads gokapi plugins from a directory.
// It supports both Go binary plugins (via host.PluginManager) and
// bridge plugins (via bridge.JavaBridge) described by manifest.json files.
//
// The directory layout uses versioned subdirectories:
//
//	{dir}/{packName}/{version}/  — contains manifest, JARs, or binaries
package loader

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/plugin/bridge"
	"github.com/gokapi/gokapi/core/plugin/host"
	pluginreg "github.com/gokapi/gokapi/core/plugin/registry"
	"github.com/gokapi/gokapi/core/preset"
	"github.com/gokapi/gokapi/core/registry"
)

// OriginalContentSetter is an alias for format.OriginalContentSetter.
// Deprecated: Use format.OriginalContentSetter directly.
type OriginalContentSetter = format.OriginalContentSetter

// SourcePathSetter is an alias for format.SourcePathSetter.
// Deprecated: Use format.SourcePathSetter directly.
type SourcePathSetter = format.SourcePathSetter

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	Name             string
	Version          string
	FrameworkVersion string // version of the underlying system (e.g., Okapi Framework version)
	Type             string // "binary" or "bridge"
	Source           string
	Formats          []string
}

// managedBridge tracks a loaded bridge plugin.
type managedBridge struct {
	cfg      bridge.BridgeConfig
	manifest *pluginreg.BundledManifest
	version  string
	formats  []string
}

// PluginLoader discovers and loads plugins from a directory.
type PluginLoader struct {
	dir     string
	manager *host.PluginManager
	pool    *bridge.BridgePool // single shared pool for all bridge plugins
	bridges []*managedBridge
	plugins []PluginInfo
	schemas *SchemaRegistry        // filter parameter schemas
	presets *preset.PresetRegistry // format and framework presets
	logger  *log.Logger

	// scanned tracks whether ScanMetadata has been called.
	scanned bool
	// bridgesLoaded tracks whether LoadBridges has been called.
	bridgesLoaded bool
	// pendingBridges holds bridge manifests discovered during ScanMetadata,
	// waiting for LoadBridges to start the actual Java processes.
	pendingBridges []pendingBridge
	// pendingBinaryDirs holds version directories for binary plugins
	// discovered during ScanMetadata, waiting for LoadBridges.
	pendingBinaryDirs []string
}

// pendingBridge holds the manifest data needed to start a bridge later.
type pendingBridge struct {
	manifest *pluginreg.BundledManifest
	dir      string
	version  string
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

// ScanMetadata discovers plugins from the configured directory and reads their
// metadata (manifests, schemas, presets) without starting any external processes.
// Bridge plugins are recorded for deferred loading via LoadBridges.
// If formatReg is non-nil, format metadata from manifest capabilities is
// registered so that "formats list" can show bridge-provided formats before
// the bridge process is started.
func (l *PluginLoader) ScanMetadata(formatReg ...*registry.FormatRegistry) error {
	var fmtReg *registry.FormatRegistry
	if len(formatReg) > 0 {
		fmtReg = formatReg[0]
	}
	if l.dir == "" {
		l.scanned = true
		return nil
	}

	info, err := os.Stat(l.dir)
	if err != nil {
		if os.IsNotExist(err) {
			l.scanned = true
			return nil
		}
		return fmt.Errorf("checking plugin directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("plugin path is not a directory: %s", l.dir)
	}

	all, err := pluginreg.ListAllInstalled(l.dir)
	if err != nil {
		l.logf("scanning versioned plugins: %v", err)
		l.scanned = true
		return nil
	}

	type versionedFmt struct {
		version string
		name    string
	}
	bareNameCandidates := make(map[string][]versionedFmt)

	for name, versions := range all {
		sort.Slice(versions, func(i, j int) bool {
			return pluginreg.CompareSemver(versions[i].Version, versions[j].Version) < 0
		})

		for _, iv := range versions {
			vDir := iv.Dir
			switch iv.InstallType {
			case "bridge":
				manifest, err := pluginreg.ReadBundledManifest(vDir)
				if err != nil {
					l.logf("reading manifest in %s: %v", vDir, err)
					continue
				}
				if manifest == nil {
					l.logf("no manifest.json in %s", vDir)
					continue
				}

				// Load schemas (NO Java needed).
				schemasDir := filepath.Join(vDir, "schemas")
				idsBefore := l.schemas.filterIDSet()
				if err := l.schemas.LoadFromDirectory(schemasDir); err != nil {
					l.logf("loading schemas from %s: %v", schemasDir, err)
				}
				idsAfter := l.schemas.filterIDSet()

				// Compute which filter IDs were added by this plugin's schemas.
				var newFilterIDs []string
				for id := range idsAfter {
					if _, existed := idsBefore[id]; !existed {
						newFilterIDs = append(newFilterIDs, id)
					}
				}
				if len(newFilterIDs) > 0 {
					l.logf("loaded %d filter schemas from %s", len(newFilterIDs), schemasDir)
				}

				// Build a lookup from capability filter ID to capability metadata.
				capByID := make(map[string]*pluginreg.Capability, len(manifest.Capabilities))
				for i := range manifest.Capabilities {
					cap := &manifest.Capabilities[i]
					if cap.Type == "format" {
						if cap.ID != "" {
							capByID[cap.ID] = cap
						}
						capByID[cap.Name] = cap
					}
				}

				// Use framework_version for format version suffix when available
				// (e.g., "1.47.0" for the underlying Okapi Framework version),
				// falling back to the plugin version (e.g., "2.12.0").
				fmtVersion := iv.FormatVersion()

				// Derive format names from schemas (preferred) or manifest capabilities.
				// Schemas carry the natural Okapi filter ID (e.g., "okf_html")
				// which is more useful than the synthesized "okapi-bridge-html".
				var formats []string
				if len(newFilterIDs) > 0 {
					sort.Strings(newFilterIDs)
					for _, filterID := range newFilterIDs {
						versionedName := filterID + "@" + fmtVersion
						formats = append(formats, versionedName)

						if fmtReg != nil {
							schema, _ := l.schemas.GetSchema(filterID)
							if schema != nil {
								info := registry.FormatInfo{
									DisplayName: schema.Title,
									MimeTypes:   schema.FilterMeta.MimeTypes,
									Extensions:  schema.FilterMeta.Extensions,
									Source:      manifest.Name,
								}
								if cap := capByID[filterID]; cap != nil {
									info.HasReader = cap.HasCapability("read")
									info.HasWriter = cap.HasCapability("write")
								}
								fmtReg.RegisterFormatInfo(versionedName, info)
							}
						}

						bareNameCandidates[filterID] = append(bareNameCandidates[filterID], versionedFmt{
							version: fmtVersion,
							name:    versionedName,
						})
					}
				} else {
					// Fallback: use manifest capabilities when no schemas available.
					for _, cap := range manifest.Capabilities {
						if cap.Type != "format" {
							continue
						}
						baseFmtName := manifest.Name + "-" + sanitizeFilterName(cap.Name)
						versionedName := baseFmtName + "@" + fmtVersion
						formats = append(formats, versionedName)

						if fmtReg != nil {
							fmtReg.RegisterFormatInfo(versionedName, registry.FormatInfo{
								DisplayName: cap.DisplayName,
								MimeTypes:   cap.MimeTypes,
								Extensions:  cap.Extensions,
								Source:      manifest.Name,
								HasReader:   cap.HasCapability("read"),
								HasWriter:   cap.HasCapability("write"),
							})
						}

						bareNameCandidates[baseFmtName] = append(bareNameCandidates[baseFmtName], versionedFmt{
							version: fmtVersion,
							name:    versionedName,
						})
					}
				}
				sort.Strings(formats)

				l.plugins = append(l.plugins, PluginInfo{
					Name:             manifest.Name,
					Version:          iv.Version,
					FrameworkVersion: iv.FrameworkVersion,
					Type:             "bridge",
					Source:           vDir,
					Formats:          formats,
				})

				l.pendingBridges = append(l.pendingBridges, pendingBridge{
					manifest: manifest,
					dir:      vDir,
					version:  fmtVersion,
				})

			case "binary":
				l.plugins = append(l.plugins, PluginInfo{
					Name:    name,
					Version: iv.Version,
					Type:    "binary",
					Source:  vDir,
				})
				l.pendingBinaryDirs = append(l.pendingBinaryDirs, vDir)
			}
		}
	}

	// Register bare-name format info aliases pointing to the latest version.
	// E.g., "okf_html" → same info as "okf_html@2.8.0" (the latest).
	if fmtReg != nil {
		for baseName, candidates := range bareNameCandidates {
			best := candidates[0]
			for _, c := range candidates[1:] {
				if pluginreg.CompareSemver(c.version, best.version) > 0 {
					best = c
				}
			}
			if info := fmtReg.FormatInfo(best.name); info != nil {
				fmtReg.RegisterFormatInfo(baseName, *info)
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

	l.scanned = true
	return nil
}

// LoadBridges starts bridge processes and registers format readers/writers
// for all bridge plugins discovered by ScanMetadata. If ScanMetadata has not
// been called, LoadBridges calls it first.
// This must be called before any file-processing command that needs bridge formats.
func (l *PluginLoader) LoadBridges(formatReg *registry.FormatRegistry, toolReg *registry.ToolRegistry) error {
	if l.bridgesLoaded {
		return nil
	}
	if !l.scanned {
		if err := l.ScanMetadata(); err != nil {
			return err
		}
	}
	l.bridgesLoaded = true

	if len(l.pendingBridges) == 0 && len(l.pendingBinaryDirs) == 0 {
		return nil
	}

	l.manager = host.NewPluginManager(l.logger)

	// Load binary plugins.
	for _, vDir := range l.pendingBinaryDirs {
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
	}
	l.pendingBinaryDirs = nil

	type versionedFormat struct {
		version string
		name    string
	}
	bareNameCandidates := make(map[string][]versionedFormat)

	for _, pb := range l.pendingBridges {
		formats, err := l.loadBridge(pb.manifest, pb.dir, pb.version, formatReg)
		if err != nil {
			l.logf("loading bridge %s: %v", pb.dir, err)
			continue
		}
		for _, fmtName := range formats {
			baseName := fmtName
			if idx := strings.LastIndex(fmtName, "@"); idx > 0 {
				baseName = fmtName[:idx]
			}
			bareNameCandidates[baseName] = append(bareNameCandidates[baseName], versionedFormat{
				version: pb.version,
				name:    fmtName,
			})
		}
	}
	l.pendingBridges = nil

	// Register bare-name aliases pointing to the latest version.
	if formatReg != nil {
		for baseName, candidates := range bareNameCandidates {
			best := candidates[0]
			for _, c := range candidates[1:] {
				if pluginreg.CompareSemver(c.version, best.version) > 0 {
					best = c
				}
			}
			if !formatReg.HasReader(baseName) {
				if rf := formatReg.ReaderFactory(best.name); rf != nil {
					formatReg.RegisterReader(baseName, rf)
					if info := formatReg.FormatInfo(best.name); info != nil {
						formatReg.SetFormatSource(baseName, info.Source)
					}
				}
			}
			if !formatReg.HasWriter(baseName) {
				if wf := formatReg.WriterFactory(best.name); wf != nil {
					formatReg.RegisterWriter(baseName, wf)
				}
			}
		}
	}

	return nil
}

// LoadAll discovers and loads all plugins from the configured directory.
// It scans for versioned subdirectories ({dir}/{name}/{version}/) and loads
// bridge manifests and binary plugins from each version directory.
// For each plugin name, both versioned format names (e.g., "okapi-html@1.46.0")
// and bare aliases (pointing to the latest version) are registered.
// If the directory does not exist, this is a no-op.
//
// LoadAll is equivalent to calling ScanMetadata followed by LoadBridges.
// Callers that don't need bridge formats immediately should call ScanMetadata
// alone and defer LoadBridges until needed.
func (l *PluginLoader) LoadAll(formatReg *registry.FormatRegistry, toolReg *registry.ToolRegistry) error {
	if err := l.ScanMetadata(formatReg); err != nil {
		return err
	}
	return l.LoadBridges(formatReg, toolReg)
}

// BridgesLoaded reports whether bridge plugins have been started.
func (l *PluginLoader) BridgesLoaded() bool {
	return l.bridgesLoaded
}

func (l *PluginLoader) loadBridge(manifest *pluginreg.BundledManifest, versionDir, version string, formatReg *registry.FormatRegistry) ([]string, error) {
	// Build BridgeConfig from manifest fields.
	command := manifest.Command
	if command == "" {
		command = "java"
	}

	// Resolve relative paths in args against the version directory.
	args := make([]string, len(manifest.Args))
	for i, arg := range manifest.Args {
		if !filepath.IsAbs(arg) && (strings.HasSuffix(arg, ".jar") || strings.HasSuffix(arg, ".exe")) {
			args[i] = filepath.Join(versionDir, arg)
		} else {
			args[i] = arg
		}
	}

	// Parse timeouts with defaults.
	startupTimeout := bridge.DefaultStartupTimeout
	if manifest.StartupTimeout != "" {
		if d, err := time.ParseDuration(manifest.StartupTimeout); err == nil {
			startupTimeout = d
		}
	}
	commandTimeout := bridge.DefaultCommandTimeout
	if manifest.CommandTimeout != "" {
		if d, err := time.ParseDuration(manifest.CommandTimeout); err == nil {
			commandTimeout = d
		}
	}

	cfg := bridge.BridgeConfig{
		Command:        command,
		Args:           args,
		Env:            manifest.Env,
		StartupTimeout: startupTimeout,
		CommandTimeout: commandTimeout,
	}

	// Lazily create the shared pool on first bridge load.
	if l.pool == nil {
		l.pool = bridge.NewBridgePool(runtime.NumCPU(), l.logger)
	}

	// Start the first bridge for filter discovery, then seed it into the shared pool.
	b := bridge.NewJavaBridge(cfg, l.logger)
	if err := b.Start(); err != nil {
		return nil, fmt.Errorf("starting bridge %q: %w", manifest.Name, err)
	}

	filters, err := b.ListFilters()
	if err != nil {
		_ = b.Stop()
		return nil, fmt.Errorf("listing filters from bridge %q: %w", manifest.Name, err)
	}

	l.pool.Seed(b)

	mb := &managedBridge{
		cfg:      cfg,
		manifest: manifest,
		version:  version,
	}

	sharedPool := l.pool
	bridgeCfg := cfg

	var formats []string
	for _, f := range filters.Filters {
		// Use the natural Okapi filter ID (e.g., "okf_html") from the bridge,
		// falling back to a synthesized name for older bridges.
		baseFmtName := f.FilterID
		if baseFmtName == "" {
			baseFmtName = manifest.Name + "-" + sanitizeFilterName(f.Name)
		}
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
			formatReg.SetFormatSource(versionedName, manifest.Name)
		}

		l.logf("registered bridge format: %s (filter: %s)", versionedName, filterClass)
	}

	l.bridges = append(l.bridges, mb)

	// Update the existing PluginInfo entry (added by ScanMetadata) with
	// the actual format list discovered from the bridge, or add a new entry
	// if loadBridge was called directly via LoadAll.
	updated := false
	for i := range l.plugins {
		if l.plugins[i].Name == manifest.Name && l.plugins[i].Version == version && l.plugins[i].Type == "bridge" {
			l.plugins[i].Formats = mb.formats
			updated = true
			break
		}
	}
	if !updated {
		l.plugins = append(l.plugins, PluginInfo{
			Name:    manifest.Name,
			Version: version,
			Type:    "bridge",
			Source:  versionDir,
			Formats: mb.formats,
		})
	}

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
