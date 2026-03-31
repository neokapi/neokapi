package cache

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/format/schema"
	pluginreg "github.com/neokapi/neokapi/core/plugin/registry"
	"github.com/neokapi/neokapi/core/preset"
	"gopkg.in/yaml.v3"
)

// skipFilters are bridge filter IDs that should not be cached.
var skipFilters = map[string]bool{
	"okf_autoxliff": true,
}

// Build performs the expensive directory scan and builds a PluginCache.
// This is called at install/update/remove time, NOT at runtime.
func Build(pluginDir string, logger *log.Logger) (*PluginCache, error) {
	if pluginDir == "" {
		return &PluginCache{Version: CacheVersion}, nil
	}

	info, err := os.Stat(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &PluginCache{Version: CacheVersion}, nil
		}
		return nil, fmt.Errorf("checking plugin directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("plugin path is not a directory: %s", pluginDir)
	}

	all, err := pluginreg.ListAllInstalled(pluginDir)
	if err != nil {
		logf(logger, "scanning plugins: %v", err)
		return &PluginCache{Version: CacheVersion}, nil
	}

	schemaReg := schema.NewSchemaRegistry()
	presetReg := preset.NewPresetRegistry()

	var plugins []CachedPlugin

	type versionedFmt struct {
		version string
		format  CachedFormat
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
				cp, fmts, err := buildBridgePlugin(iv, vDir, schemaReg, presetReg, logger)
				if err != nil {
					logf(logger, "building cache for %s/%s: %v", name, iv.Version, err)
					continue
				}
				plugins = append(plugins, cp)

				fmtVersion := iv.FormatVersion()
				for _, cf := range fmts {
					bareNameCandidates[cf.BaseName] = append(bareNameCandidates[cf.BaseName], versionedFmt{
						version: fmtVersion,
						format:  cf,
					})
				}

			case "binary":
				cp := buildBinaryPlugin(iv, vDir)
				plugins = append(plugins, cp)
			}
		}
	}

	// Scan for presets.yaml files.
	for _, versions := range all {
		for _, iv := range versions {
			presetsPath := filepath.Join(iv.Dir, "presets.yaml")
			if _, err := os.Stat(presetsPath); err == nil {
				if err := loadPresetsFromFile(presetsPath, presetReg, iv.Dir); err != nil {
					logf(logger, "loading presets from %s: %v", presetsPath, err)
				}
			}
		}
	}

	// Extract presets from schemas.
	schemaReg.ExtractPresets(presetReg)

	// Build bare-name alias entries in each plugin's format list.
	// These point to the latest version's format metadata.
	for baseName, candidates := range bareNameCandidates {
		best := candidates[0]
		for _, c := range candidates[1:] {
			if pluginreg.CompareSemver(c.version, best.version) > 0 {
				best = c
			}
		}
		alias := best.format
		alias.VersionedName = baseName
		// Add alias to the same plugin that owns the best version.
		for i := range plugins {
			for _, f := range plugins[i].Formats {
				if f.VersionedName == best.format.VersionedName {
					plugins[i].Formats = append(plugins[i].Formats, alias)
					break
				}
			}
		}
	}

	// Discover docs directory from any plugin that provides one.
	var docsDir string
	for _, versions := range all {
		for _, iv := range versions {
			d := filepath.Join(iv.Dir, "docs")
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				docsDir = d
				break
			}
		}
		if docsDir != "" {
			break
		}
	}

	cache := &PluginCache{
		Version: CacheVersion,
		Plugins: plugins,
		Schemas: collectSchemas(schemaReg),
		Presets: collectPresets(presetReg),
		DocsDir: docsDir,
	}
	return cache, nil
}

// RebuildAndWrite builds a fresh cache and writes it to disk.
func RebuildAndWrite(pluginDir string, logger *log.Logger) error {
	c, err := Build(pluginDir, logger)
	if err != nil {
		return err
	}
	return Write(pluginDir, c)
}

func buildBridgePlugin(iv pluginreg.InstalledVersion, vDir string, schemaReg *schema.SchemaRegistry, presetReg *preset.PresetRegistry, logger *log.Logger) (CachedPlugin, []CachedFormat, error) {
	manifest, err := pluginreg.ReadBundledManifest(vDir)
	if err != nil {
		return CachedPlugin{}, nil, fmt.Errorf("reading manifest: %w", err)
	}
	if manifest == nil {
		return CachedPlugin{}, nil, fmt.Errorf("no manifest.json in %s", vDir)
	}

	// Load schemas.
	schemasDir := filepath.Join(vDir, "schemas")
	idsBefore := schemaReg.FilterIDSet()
	if err := schemaReg.LoadFromDirectory(schemasDir); err != nil {
		logf(logger, "loading schemas from %s: %v", schemasDir, err)
	}
	idsAfter := schemaReg.FilterIDSet()

	var newFilterIDs []string
	for id := range idsAfter {
		if _, existed := idsBefore[id]; !existed {
			newFilterIDs = append(newFilterIDs, id)
		}
	}

	// Build capability lookup.
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

	var formats []CachedFormat
	if len(newFilterIDs) > 0 {
		sort.Strings(newFilterIDs)
		for _, filterID := range newFilterIDs {
			if skipFilters[filterID] {
				continue
			}
			versionedName := filterID + "@" + fmtVersion

			var cf CachedFormat
			cf.VersionedName = versionedName
			cf.BaseName = filterID
			cf.Source = manifest.Name

			if s, ok := schemaReg.GetSchema(filterID); ok {
				cf.DisplayName = s.Title
				cf.MimeTypes = s.FilterMeta.MimeTypes
				cf.Extensions = s.FilterMeta.Extensions
				cf.FilterClass = s.FilterMeta.Class
			}
			if cap := capByID[filterID]; cap != nil {
				cf.HasReader = cap.HasCapability("read")
				cf.HasWriter = cap.HasCapability("write")
			}
			formats = append(formats, cf)
		}
	} else {
		for _, cap := range manifest.Capabilities {
			if cap.Type != "format" {
				continue
			}
			baseFmtName := manifest.Name + "-" + sanitizeFilterName(cap.Name)
			versionedName := baseFmtName + "@" + fmtVersion
			formats = append(formats, CachedFormat{
				VersionedName: versionedName,
				BaseName:      baseFmtName,
				DisplayName:   cap.DisplayName,
				MimeTypes:     cap.MimeTypes,
				Extensions:    cap.Extensions,
				HasReader:     cap.HasCapability("read"),
				HasWriter:     cap.HasCapability("write"),
				Source:        manifest.Name,
			})
		}
	}

	cp := CachedPlugin{
		Name:             manifest.Name,
		Version:          iv.Version,
		FrameworkVersion: iv.FrameworkVersion,
		InstallType:      "bridge",
		PluginType:       iv.PluginType,
		Dir:              vDir,
		Formats:          formats,
		Manifest:         manifest,
	}
	return cp, formats, nil
}

func buildBinaryPlugin(iv pluginreg.InstalledVersion, vDir string) CachedPlugin {
	cp := CachedPlugin{
		Name:        iv.Name,
		Version:     iv.Version,
		InstallType: "binary",
		PluginType:  iv.PluginType,
		Dir:         vDir,
	}
	// Use capabilities from version.json if available.
	for _, cap := range iv.Capabilities {
		if cap.Type == "format" {
			cp.Formats = append(cp.Formats, CachedFormat{
				VersionedName: cap.Name,
				BaseName:      cap.Name,
				DisplayName:   cap.DisplayName,
				MimeTypes:     cap.MimeTypes,
				Extensions:    cap.Extensions,
				HasReader:     cap.HasCapability("read"),
				HasWriter:     cap.HasCapability("write"),
				Source:        iv.Name,
			})
		}
	}
	return cp
}

func collectSchemas(reg *schema.SchemaRegistry) map[string]*schema.FilterSchema {
	ids := reg.FilterIDs()
	if len(ids) == 0 {
		return nil
	}
	result := make(map[string]*schema.FilterSchema, len(ids))
	for _, id := range ids {
		if s, ok := reg.GetSchema(id); ok {
			result[id] = s
		}
	}
	return result
}

func collectPresets(reg *preset.PresetRegistry) CachedPresets {
	cp := CachedPresets{}

	for _, format := range reg.FormatNames() {
		presets := reg.ListFormatPresets(format)
		if len(presets) > 0 {
			if cp.FormatPresets == nil {
				cp.FormatPresets = make(map[string]map[string]*preset.FormatPreset)
			}
			m := make(map[string]*preset.FormatPreset, len(presets))
			for _, p := range presets {
				m[p.Name] = p
			}
			cp.FormatPresets[format] = m
		}
	}

	frameworks := reg.ListFrameworkPresets()
	if len(frameworks) > 0 {
		cp.FrameworkPresets = make(map[string]*preset.FrameworkPreset, len(frameworks))
		for _, p := range frameworks {
			cp.FrameworkPresets[p.Name] = p
		}
	}

	return cp
}

func sanitizeFilterName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

// presetManifest is the YAML structure for presets.yaml files.
type presetManifest struct {
	Kind             string                          `yaml:"kind"`
	FormatPresets    map[string]formatPresetEntry    `yaml:"format_presets,omitempty"`
	FrameworkPresets map[string]frameworkPresetEntry `yaml:"framework_presets,omitempty"`
}

type formatPresetEntry struct {
	Description string                    `yaml:"description"`
	Formats     []formatPresetFormatEntry `yaml:"formats"`
}

type formatPresetFormatEntry struct {
	Format string         `yaml:"format"`
	Config map[string]any `yaml:"config"`
}

type frameworkPresetEntry struct {
	Description   string                    `yaml:"description"`
	Mappings      []frameworkMappingEntry   `yaml:"mappings,omitempty"`
	Exclude       []string                  `yaml:"exclude,omitempty"`
	FormatPresets map[string]map[string]any `yaml:"format_presets,omitempty"`
	Flows         map[string]map[string]any `yaml:"flows,omitempty"`
}

type frameworkMappingEntry struct {
	Local      string `yaml:"local"`
	Remote     string `yaml:"remote,omitempty"`
	Format     string `yaml:"format"`
	TargetPath string `yaml:"target_path,omitempty"`
}

func loadPresetsFromFile(path string, reg *preset.PresetRegistry, source string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading presets file: %w", err)
	}

	var manifest presetManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parsing presets YAML: %w", err)
	}

	for name, entry := range manifest.FormatPresets {
		for _, f := range entry.Formats {
			reg.RegisterFormatPreset(f.Format, name, &preset.FormatPreset{
				Name:        name,
				Description: entry.Description,
				Format:      f.Format,
				Config:      f.Config,
				Source:      source,
			})
		}
	}

	for name, entry := range manifest.FrameworkPresets {
		fp := &preset.FrameworkPreset{
			Name:          name,
			Description:   entry.Description,
			Exclude:       entry.Exclude,
			FormatPresets: entry.FormatPresets,
			Flows:         entry.Flows,
			Source:        source,
		}
		for _, m := range entry.Mappings {
			fp.Mappings = append(fp.Mappings, preset.MappingTemplate{
				Local:      m.Local,
				Remote:     m.Remote,
				Format:     m.Format,
				TargetPath: m.TargetPath,
			})
		}
		reg.RegisterFrameworkPreset(name, fp)
	}

	return nil
}

func logf(logger *log.Logger, format string, args ...any) {
	if logger != nil {
		logger.Printf(format, args...)
	}
}



