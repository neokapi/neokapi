// Package loader discovers and loads neokapi plugins from a directory.
// It supports both Go binary plugins (via host.PluginManager) and
// bridge plugins (via bridge.JavaBridge) described by manifest.json files.
//
// The directory layout uses versioned subdirectories:
//
//	{dir}/{packName}/{version}/  — contains manifest, JARs, or binaries
package loader

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/leonelquinteros/gotext"

	"github.com/neokapi/neokapi/core/format"
	fmtschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	plugincache "github.com/neokapi/neokapi/core/plugin/cache"
	"github.com/neokapi/neokapi/core/plugin/host"
	pluginreg "github.com/neokapi/neokapi/core/plugin/registry"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// OriginalContentSetter is an alias for format.OriginalContentSetter.
//
// Deprecated: Use format.OriginalContentSetter directly.
type OriginalContentSetter = format.OriginalContentSetter

// skipFilters are bridge filter IDs that should not be registered.
// AutoXLIFFFilter is a delegating meta-filter that wraps XLIFFFilter/XLIFF2Filter.
// Its delegate initialization happens inside open(), but the bridge's lifecycle
// calls setFilterConfigurationMapper before open(), causing an NPE. The concrete
// okf_xliff and okf_xliff2 filters handle the same extensions and work correctly.
var skipFilters = map[string]bool{
	"okf_autoxliff": true,
}

// SourcePathSetter is an alias for format.SourcePathSetter.
//
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
	dir      string
	manager  *host.PluginManager
	registry *bridge.BridgeRegistry // single shared registry for all bridge plugins
	bridges  []*managedBridge
	plugins  []PluginInfo
	schemas  *SchemaRegistry        // filter parameter schemas
	presets  *preset.PresetRegistry // format and framework presets
	toolReg  *registry.ToolRegistry // tool registry for plugin tools (optional)
	docsDir  string                 // path to docs/ directory with per-filter/step JSON files
	logger   *log.Logger

	// disabledPlugins is a set of plugin names to skip during scan and load.
	disabledPlugins map[string]bool

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

// SetDisabledPlugins sets the plugin names to skip during scan and load.
// Must be called before ScanMetadata.
func (l *PluginLoader) SetDisabledPlugins(names map[string]bool) {
	l.disabledPlugins = names
}

// ScanMetadata reads plugin metadata from the pre-computed cache file
// ({plugin_dir}/plugin-cache.json). If the cache is missing or corrupt,
// it falls back to a full directory scan and rebuilds the cache for next time.
//
// No external processes are started. Bridge plugins are recorded for
// deferred loading via LoadBridges.
func (l *PluginLoader) ScanMetadata(formatReg ...*registry.FormatRegistry) error {
	// Clear accumulated state so ScanMetadata is idempotent when called
	// multiple times (e.g., after plugin install/remove in the desktop app).
	l.plugins = nil
	l.pendingBridges = nil

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

	// Try the pre-computed cache first (single file read, no directory scanning).
	cached, err := plugincache.Read(l.dir)
	if err == nil {
		l.logf("loaded plugin cache (%d plugins)", len(cached.Plugins))
		l.loadFromCache(cached, fmtReg)
		l.scanned = true
		return nil
	}
	l.logf("plugin cache miss: %v; falling back to full scan", err)

	// Fallback: full directory scan + rebuild cache for next time.
	if err := l.scanFromDisk(fmtReg); err != nil {
		return err
	}
	// Best-effort cache rebuild so next startup is fast.
	if rebuildErr := plugincache.RebuildAndWrite(l.dir, l.logger); rebuildErr != nil {
		l.logf("rebuilding plugin cache: %v", rebuildErr)
	}
	l.scanned = true
	return nil
}

// loadFromCache populates the loader state from a pre-computed cache.
// Zero directory scanning, zero file parsing.
func (l *PluginLoader) loadFromCache(c *plugincache.PluginCache, fmtReg *registry.FormatRegistry) {
	// Populate schemas.
	for id, s := range c.Schemas {
		l.schemas.RegisterSchema(id, s)
	}

	// Register plugin tool schemas into the tool registry.
	if l.toolReg != nil {
		for _, cp := range c.Plugins {
			if cp.Manifest == nil {
				continue
			}
			for _, cap := range cp.Manifest.Capabilities {
				if cap.Type != "tool" {
					continue
				}
				name := cap.ID
				if name == "" {
					name = cap.Name
				}
				var toolSchema *fmtschema.FormatSchema
				if s, ok := c.ToolSchemas[name]; ok {
					toolSchema = s
				}
				// Build a ComponentSchema from the capability + optional FormatSchema.
				cs := capabilityToComponentSchema(cap, toolSchema)
				l.toolReg.RegisterMetadata(registry.ToolID(name), cs, cp.Name)
			}
		}
	}

	// Populate docs directory path.
	if c.DocsDir != "" {
		l.docsDir = c.DocsDir
	}

	// Populate presets.
	for format, presets := range c.Presets.FormatPresets {
		for name, p := range presets {
			l.presets.RegisterFormatPreset(format, name, p)
		}
	}
	for name, p := range c.Presets.FrameworkPresets {
		l.presets.RegisterFrameworkPreset(name, p)
	}

	for _, cp := range c.Plugins {
		if l.disabledPlugins[cp.Name] {
			l.logf("skipping disabled plugin: %s", cp.Name)
			continue
		}

		var formats []string
		for _, cf := range cp.Formats {
			formats = append(formats, cf.VersionedName)
		}

		l.plugins = append(l.plugins, PluginInfo{
			Name:             cp.Name,
			Version:          cp.Version,
			FrameworkVersion: cp.FrameworkVersion,
			Type:             cp.InstallType,
			Source:           cp.Dir,
			Formats:          formats,
		})

		switch cp.InstallType {
		case "bridge":
			if cp.Manifest != nil {
				fmtVersion := cp.FrameworkVersion
				if fmtVersion == "" {
					fmtVersion = cp.Version
				}
				l.pendingBridges = append(l.pendingBridges, pendingBridge{
					manifest: cp.Manifest,
					dir:      cp.Dir,
					version:  fmtVersion,
				})
			}
		case "binary":
			l.pendingBinaryDirs = append(l.pendingBinaryDirs, cp.Dir)
		}

		// Register format metadata with the format registry.
		if fmtReg != nil {
			for _, cf := range cp.Formats {
				fmtReg.RegisterFormatInfo(registry.FormatID(cf.VersionedName), registry.FormatInfo{
					DisplayName: cf.DisplayName,
					MimeTypes:   cf.MimeTypes,
					Extensions:  cf.Extensions,
					Source:      cf.Source,
					HasReader:   cf.HasReader,
					HasWriter:   cf.HasWriter,
				})
			}
		}
	}
}

// scanFromDisk performs the full directory scan (fallback when cache is missing).
func (l *PluginLoader) scanFromDisk(fmtReg *registry.FormatRegistry) error {
	all, err := pluginreg.ListAllInstalled(l.dir)
	if err != nil {
		l.logf("scanning versioned plugins: %v", err)
		return nil
	}

	type versionedFmt struct {
		version string
		name    string
	}
	bareNameCandidates := make(map[string][]versionedFmt)

	for name, versions := range all {
		if l.disabledPlugins[name] {
			l.logf("skipping disabled plugin: %s", name)
			continue
		}

		slices.SortFunc(versions, func(a, b pluginreg.InstalledVersion) int {
			return pluginreg.CompareSemver(a.Version, b.Version)
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

				schemasDir := filepath.Join(vDir, "schemas")
				idsBefore := l.schemas.FormatIDSet()
				if err := l.schemas.LoadFromDirectory(schemasDir); err != nil {
					l.logf("loading schemas from %s: %v", schemasDir, err)
				}
				idsAfter := l.schemas.FormatIDSet()

				// Discover documentation directory if present.
				docsPath := filepath.Join(vDir, "docs")
				if info, err := os.Stat(docsPath); err == nil && info.IsDir() {
					l.docsDir = docsPath
				}

				var newFilterIDs []string
				for _, id := range idsAfter.Items() {
					if !idsBefore.Contains(id) {
						newFilterIDs = append(newFilterIDs, id)
					}
				}

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

				fmtVersion := iv.FormatVersion()

				var formats []string
				if len(newFilterIDs) > 0 {
					slices.Sort(newFilterIDs)
					for _, filterID := range newFilterIDs {
						if skipFilters[filterID] {
							continue
						}
						versionedName := filterID + "@" + fmtVersion
						formats = append(formats, versionedName)

						if fmtReg != nil {
							schema, _ := l.schemas.GetSchema(filterID)
							if schema != nil {
								info := registry.FormatInfo{
									DisplayName: schema.Title,
									MimeTypes:   schema.FormatMeta.MimeTypes,
									Extensions:  schema.FormatMeta.Extensions,
									Source:      manifest.Name,
								}
								if cap := capByID[filterID]; cap != nil {
									info.HasReader = cap.HasCapability("read")
									info.HasWriter = cap.HasCapability("write")
								}
								fmtReg.RegisterFormatInfo(registry.FormatID(versionedName), info)
							}
						}

						bareNameCandidates[filterID] = append(bareNameCandidates[filterID], versionedFmt{
							version: fmtVersion,
							name:    versionedName,
						})
					}
				} else {
					for _, cap := range manifest.Capabilities {
						if cap.Type != "format" {
							continue
						}
						baseFmtName := manifest.Name + "-" + sanitizeFilterName(cap.Name)
						versionedName := baseFmtName + "@" + fmtVersion
						formats = append(formats, versionedName)

						if fmtReg != nil {
							fmtReg.RegisterFormatInfo(registry.FormatID(versionedName), registry.FormatInfo{
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
				slices.Sort(formats)

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

	if fmtReg != nil {
		for baseName, candidates := range bareNameCandidates {
			best := candidates[0]
			for _, c := range candidates[1:] {
				if pluginreg.CompareSemver(c.version, best.version) > 0 {
					best = c
				}
			}
			if info := fmtReg.FormatInfo(registry.FormatID(best.name)); info != nil {
				fmtReg.RegisterFormatInfo(registry.FormatID(baseName), *info)
			}
		}
	}

	l.schemas.ExtractPresets(l.presets)

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
		pattern := filepath.Join(vDir, "neokapi-plugin-*")
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
		formats, err := l.loadBridge(pb.manifest, pb.dir, pb.version, formatReg, toolReg)
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
			if !formatReg.HasReader(registry.FormatID(baseName)) {
				if rf := formatReg.ReaderFactory(registry.FormatID(best.name)); rf != nil {
					var sig format.FormatSignature
					var displayName string
					if info := formatReg.FormatInfo(registry.FormatID(best.name)); info != nil {
						sig = format.FormatSignature{
							MIMETypes:  info.MimeTypes,
							Extensions: info.Extensions,
						}
						displayName = info.DisplayName
					}
					formatReg.RegisterReader(registry.FormatID(baseName), rf, sig, displayName)
					if info := formatReg.FormatInfo(registry.FormatID(best.name)); info != nil {
						formatReg.SetFormatSource(registry.FormatID(baseName), info.Source)
					}
				}
			}
			if !formatReg.HasWriter(registry.FormatID(baseName)) {
				if wf := formatReg.WriterFactory(registry.FormatID(best.name)); wf != nil {
					formatReg.RegisterWriter(registry.FormatID(baseName), wf)
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

// buildBridgeConfig builds a BridgeConfig from a manifest and version directory.
func buildBridgeConfig(manifest *pluginreg.BundledManifest, versionDir string) bridge.BridgeConfig {
	command := manifest.Command
	if command == "" {
		command = "java"
	}

	// Resolve relative paths in args against the version directory.
	// Prepend JVM heap size if not already set in the manifest args.
	var args []string
	hasXmx := false
	for _, arg := range manifest.Args {
		if strings.HasPrefix(arg, "-Xmx") {
			hasXmx = true
		}
	}
	if command == "java" && !hasXmx {
		heap := os.Getenv("KAPI_BRIDGE_HEAP")
		if heap == "" {
			heap = "16g"
		}
		args = append(args, "-Xmx"+heap)
		// Skip Netty's slow MAC address enumeration for channel IDs.
		// Without this, DefaultChannelId init takes ~5s on macOS.
		args = append(args, "-Dio.netty.machineId=00:00:00:00:00:01")
	}
	for _, arg := range manifest.Args {
		if !filepath.IsAbs(arg) && (strings.HasSuffix(arg, ".jar") || strings.HasSuffix(arg, ".exe")) {
			args = append(args, filepath.Join(versionDir, arg))
		} else {
			args = append(args, arg)
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

	return bridge.BridgeConfig{
		Command:        command,
		Args:           args,
		Env:            manifest.Env,
		StartupTimeout: startupTimeout,
		CommandTimeout: commandTimeout,
	}
}

func (l *PluginLoader) loadBridge(manifest *pluginreg.BundledManifest, versionDir, version string, formatReg *registry.FormatRegistry, toolReg ...*registry.ToolRegistry) ([]string, error) {
	cfg := buildBridgeConfig(manifest, versionDir)

	// Lazily create the shared registry on first bridge load.
	// No JVM is started here — bridges are created on-demand by the registry
	// when format readers/writers first acquire them.
	if l.registry == nil {
		l.registry = bridge.NewBridgeRegistry(runtime.NumCPU(), 8, l.logger)
		// Enable daemon mode if KAPI_BRIDGE_DAEMON is set.
		if os.Getenv("KAPI_BRIDGE_DAEMON") == "1" {
			timeout := 30 * time.Second
			if v := os.Getenv("KAPI_BRIDGE_IDLE_TIMEOUT"); v != "" {
				if d, err := time.ParseDuration(v); err == nil {
					timeout = d
				}
			}
			l.registry.SetDaemonMode(true, timeout)
			l.logf("bridge daemon mode enabled (idle timeout: %s)", timeout)
		}
	}

	mb := &managedBridge{
		cfg:      cfg,
		manifest: manifest,
		version:  version,
	}

	sharedRegistry := l.registry
	bridgeCfg := cfg

	// Register formats using manifest capabilities + schema metadata.
	// Filter class names come from schemas (loaded during ScanMetadata),
	// eliminating the need to start a JVM and call ListFormats.
	var formats []string
	for _, cap := range manifest.Capabilities {
		if cap.Type != "format" {
			continue
		}

		filterID := cap.ID
		if filterID == "" {
			continue
		}
		if skipFilters[filterID] {
			continue
		}

		// Look up the Java filter class from the schema registry.
		// Schemas are loaded from disk during ScanMetadata — no JVM needed.
		var filterClass string
		if s, ok := l.schemas.GetSchema(filterID); ok && s.FormatMeta.Class != "" {
			filterClass = s.FormatMeta.Class
		}
		if filterClass == "" {
			l.logf("skipping bridge format %s: no filter class in schema", filterID)
			continue
		}

		versionedName := filterID + "@" + version
		mb.formats = append(mb.formats, versionedName)
		formats = append(formats, versionedName)

		if formatReg != nil {
			// Build FormatSignature from schema metadata so the reader
			// doesn't need to query the JVM for format detection info.
			var sig format.FormatSignature
			if s, ok := l.schemas.GetSchema(filterID); ok {
				sig = format.FormatSignature{
					MIMETypes:  s.FormatMeta.MimeTypes,
					Extensions: s.FormatMeta.Extensions,
				}
			}

			formatReg.RegisterReader(registry.FormatID(versionedName), func() format.DataFormatReader {
				return bridge.NewBridgeFormatReader(sharedRegistry, bridgeCfg, filterClass, sig)
			}, sig, "")
			// No separate writer registration — bridge formats use BridgeProcessor
			// for the single-pass pipeline (Go acts as an Okapi step).
			formatReg.SetFormatSource(registry.FormatID(versionedName), manifest.Name)
		}

		l.logf("registered bridge format: %s (filter: %s)", versionedName, filterClass)
	}

	l.bridges = append(l.bridges, mb)

	// Register step tools from schemas/steps/ directory.
	var tReg *registry.ToolRegistry
	if len(toolReg) > 0 {
		tReg = toolReg[0]
	}
	if tReg != nil {
		l.loadBridgeStepTools(versionDir, sharedRegistry, bridgeCfg, tReg, manifest.Name)
	}

	// Update the existing PluginInfo entry (added by ScanMetadata) with
	// the actual format list, or add a new entry if loadBridge was called
	// directly via LoadAll.
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

// loadBridgeStepTools scans schemas/steps/ within a bridge plugin directory
// for step schema JSON files and registers each as a tool.Tool.
//
// Schema extraction preserves Okapi naming (e.g., step ID "search-and-replace").
// Tool names use the step ID directly — no prefix is added.
func (l *PluginLoader) loadBridgeStepTools(versionDir string, reg *bridge.BridgeRegistry, cfg bridge.BridgeConfig, toolReg *registry.ToolRegistry, source string) {
	stepsDir := filepath.Join(versionDir, "schemas", "steps")
	entries, err := os.ReadDir(stepsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(stepsDir, entry.Name()))
		if err != nil {
			l.logf("reading step schema %s: %v", entry.Name(), err)
			continue
		}

		var cs schema.ComponentSchema
		if err := json.Unmarshal(data, &cs); err != nil {
			l.logf("parsing step schema %s: %v", entry.Name(), err)
			continue
		}
		if cs.ToolMeta == nil || cs.ToolMeta.ID == "" {
			continue
		}

		stepClass := cs.ToolMeta.ID
		toolName := cs.ID
		if toolName == "" {
			toolName = cs.ToolMeta.ID
		}

		// Capture for closure.
		schemaRef := &cs
		stepClassRef := stepClass
		cfgRef := cfg
		desc := cs.Description
		if desc == "" && cs.ToolMeta != nil {
			desc = cs.ToolMeta.Description
		}

		toolReg.RegisterWithSchema(registry.ToolID(toolName), func() tool.Tool {
			return bridge.NewBridgeStepTool(reg, cfgRef, stepClassRef, toolName, desc, schemaRef)
		}, schemaRef)

		l.logf("registered bridge step tool: %s (source: %s)", toolName, source)
	}
}

// SetToolRegistry sets the tool registry that plugin tools will be registered into
// during ScanMetadata. Call before ScanMetadata.
func (l *PluginLoader) SetToolRegistry(reg *registry.ToolRegistry) {
	l.toolReg = reg
}

// capabilityToComponentSchema builds a ComponentSchema from a plugin capability
// and an optional FormatSchema (loaded from schemas/steps/). The FormatSchema
// provides property definitions and groups; the capability provides metadata.
func capabilityToComponentSchema(cap pluginreg.Capability, fs *fmtschema.FormatSchema) *schema.ComponentSchema {
	cs := &schema.ComponentSchema{
		Type: "object",
		ToolMeta: &schema.ToolMeta{
			ID:          cap.ID,
			Category:    cap.Category,
			DisplayName: cap.DisplayName,
			Description: cap.Description,
			Inputs:      cap.Inputs,
			Outputs:     cap.Outputs,
			Tags:        cap.Tags,
			Requires:    cap.Requires,
		},
	}
	cs.Title = cap.DisplayName
	cs.Description = cap.Description

	// Merge schema properties from FormatSchema if available.
	if fs != nil {
		cs.ID = fs.ID
		cs.Version = fs.Version
		if fs.Title != "" {
			cs.Title = fs.Title
		}
		if fs.Description != "" {
			cs.Description = fs.Description
		}

		// If the FormatSchema has pre-built RawJSON (loaded from a schema file),
		// Convert FormatSchema properties/groups via JSON round-trip.
		data, err := json.Marshal(fs)
		if err == nil {
			var proxy struct {
				Groups     []schema.ParameterGroup          `json:"ui:groups"`
				Properties map[string]schema.PropertySchema `json:"properties"`
			}
			if json.Unmarshal(data, &proxy) == nil {
				cs.Groups = proxy.Groups
				cs.Properties = proxy.Properties
			}
		}
	}

	return cs
}

// Plugins returns information about all loaded plugins.
func (l *PluginLoader) Plugins() []PluginInfo {
	return l.plugins
}

// Dir returns the plugin directory path.
func (l *PluginLoader) Dir() string {
	return l.dir
}

// I18nCatalogs returns the set of gettext MO catalogs contributed by
// installed plugins for the given locale. Each plugin that ships a
// compiled catalog at `<pluginDir>/<name>/<version>/i18n/<locale>.mo`
// (or at a custom path set via PluginManifest.I18nDir) gets one entry.
// Missing files are silently skipped — absence of a translation is
// normal, not an error. Callers (typically cli/app.go during Init) merge
// the returned slice into i18n.ResolveOptions.PluginCatalogs.
//
// Safe to call after ScanMetadata. Returns nil and no error when dir is
// empty or no plugin ships a matching catalog.
func (l *PluginLoader) I18nCatalogs(locale model.LocaleID) ([]*gotext.Mo, error) {
	if locale.IsEmpty() || locale == "en" || l.dir == "" {
		return nil, nil
	}
	var catalogs []*gotext.Mo
	for _, p := range l.plugins {
		if p.Name == "" || p.Version == "" {
			continue
		}
		// Convention: pluginRoot/<name>/<version>/i18n/<locale>.mo.
		// The I18nDir override on PluginManifest isn't threaded through
		// PluginInfo yet — if a plugin needs a non-default dir, we read
		// that during ScanMetadata and surface it via PluginInfo in a
		// follow-up. For v1 every plugin we ship (and okapi-bridge)
		// uses the convention.
		versionDir := filepath.Join(l.dir, p.Name, p.Version)
		path := filepath.Join(versionDir, "i18n", string(locale)+".mo")
		mo, err := loadMoFile(path)
		if err != nil {
			l.logf("plugin %s: loading i18n catalog %s: %v", p.Name, path, err)
			continue
		}
		if mo != nil {
			catalogs = append(catalogs, mo)
		}
	}
	return catalogs, nil
}

// loadMoFile reads and parses a gettext MO file. Returns (nil, nil) when
// the file is absent; returns an error only on I/O or parse failure.
func loadMoFile(path string) (*gotext.Mo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	mo := gotext.NewMo()
	mo.Parse(data)
	return mo, nil
}

// Schemas returns the schema registry for filter parameters.
func (l *PluginLoader) Schemas() *SchemaRegistry {
	return l.schemas
}

// Presets returns the preset registry.
func (l *PluginLoader) Presets() *preset.PresetRegistry {
	return l.presets
}

// DocsDir returns the path to the docs/ directory, or "" if unavailable.
func (l *PluginLoader) DocsDir() string {
	return l.docsDir
}

// FilterDoc reads and returns documentation for a single filter by ID.
// Returns nil if the docs directory is unavailable or the filter has no docs.
func (l *PluginLoader) FilterDoc(filterID string) json.RawMessage {
	if l.docsDir == "" {
		return nil
	}
	path := filepath.Join(l.docsDir, "filters", filterID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		// Try alias resolution via metadata.json.
		if aliased := l.resolveAlias(filterID); aliased != "" {
			path = filepath.Join(l.docsDir, "filters", aliased+".json")
			data, err = os.ReadFile(path)
		}
		if err != nil {
			return nil
		}
	}
	return data
}

// StepDoc reads and returns documentation for a single pipeline step by ID.
// Returns nil if the docs directory is unavailable or the step has no docs.
func (l *PluginLoader) StepDoc(stepID string) json.RawMessage {
	if l.docsDir == "" {
		return nil
	}
	path := filepath.Join(l.docsDir, "steps", stepID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

// DocsMetadata reads and returns the docs metadata (aliases, wiki URL, etc).
// Returns nil if unavailable.
func (l *PluginLoader) DocsMetadata() json.RawMessage {
	if l.docsDir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(l.docsDir, "metadata.json"))
	if err != nil {
		return nil
	}
	return data
}

// resolveAlias looks up a filter ID alias in metadata.json.
func (l *PluginLoader) resolveAlias(filterID string) string {
	meta := l.DocsMetadata()
	if meta == nil {
		return ""
	}
	var m struct {
		Aliases map[string]string `json:"aliases"`
	}
	if err := json.Unmarshal(meta, &m); err != nil {
		return ""
	}
	return m.Aliases[filterID]
}

// ListFilterDocs returns the IDs of all filters that have documentation.
func (l *PluginLoader) ListFilterDocs() []string {
	if l.docsDir == "" {
		return nil
	}
	dir := filepath.Join(l.docsDir, "filters")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".json"))
	}
	return ids
}

// ListStepDocs returns the IDs of all steps that have documentation.
func (l *PluginLoader) ListStepDocs() []string {
	if l.docsDir == "" {
		return nil
	}
	dir := filepath.Join(l.docsDir, "steps")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".json"))
	}
	return ids
}

// Registry returns the shared bridge registry, or nil if no bridges are loaded.
func (l *PluginLoader) Registry() *bridge.BridgeRegistry {
	return l.registry
}

// WarmupBridges eagerly starts one JVM per bridge configuration so it's
// ready when files arrive. Call this before concurrent file processing
// to amortize JVM startup cost.
func (l *PluginLoader) WarmupBridges() {
	if l.registry == nil {
		return
	}
	for _, mb := range l.bridges {
		_ = l.registry.Warmup(mb.cfg)
	}
}

// Shutdown stops all plugin processes.
func (l *PluginLoader) Shutdown() {
	if l.manager != nil {
		l.manager.Shutdown()
	}
	if l.registry != nil {
		l.registry.Shutdown()
		l.registry = nil
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
