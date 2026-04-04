package cache

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
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
	toolSchemaReg := schema.NewSchemaRegistry()
	presetReg := preset.NewPresetRegistry()

	var plugins []CachedPlugin

	type versionedFmt struct {
		version string
		format  CachedFormat
	}
	bareNameCandidates := make(map[string][]versionedFmt)

	for name, versions := range all {
		slices.SortFunc(versions, func(a, b pluginreg.InstalledVersion) int {
			return pluginreg.CompareSemver(a.Version, b.Version)
		})

		for _, iv := range versions {
			vDir := iv.Dir
			switch iv.InstallType {
			case "bridge":
				cp, fmts, err := buildBridgePlugin(iv, vDir, schemaReg, toolSchemaReg, presetReg, logger)
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
		Version:     CacheVersion,
		Plugins:     plugins,
		Schemas:     collectSchemas(schemaReg),
		ToolSchemas: collectSchemas(toolSchemaReg),
		Presets:     collectPresets(presetReg),
		DocsDir:     docsDir,
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

func buildBridgePlugin(iv pluginreg.InstalledVersion, vDir string, schemaReg *schema.SchemaRegistry, toolSchemaReg *schema.SchemaRegistry, presetReg *preset.PresetRegistry, logger *log.Logger) (CachedPlugin, []CachedFormat, error) {
	manifest, err := pluginreg.ReadBundledManifest(vDir)
	if err != nil {
		return CachedPlugin{}, nil, fmt.Errorf("reading manifest: %w", err)
	}
	if manifest == nil {
		return CachedPlugin{}, nil, fmt.Errorf("no manifest.json in %s", vDir)
	}

	// Load schemas from explicit capability paths.
	fmtVersion := iv.FormatVersion()
	var formats []CachedFormat

	for _, cap := range manifest.Capabilities {
		switch cap.Type {
		case "format":
			formatID := cap.ID
			if formatID == "" {
				formatID = cap.Name
			}
			if skipFilters[formatID] {
				continue
			}

			// Load schema from explicit path if provided, otherwise skip.
			if cap.Schema != "" {
				schemaPath := filepath.Join(vDir, cap.Schema)
				if err := schemaReg.LoadSchemaFile(schemaPath, formatID); err != nil {
					logf(logger, "loading format schema %s: %v", schemaPath, err)
				}
			}

			// Load presets from presets directory if provided.
			if cap.PresetsDir != "" {
				presetsPath := filepath.Join(vDir, cap.PresetsDir)
				if info, err := os.Stat(presetsPath); err == nil && info.IsDir() {
					loadPresetsFromDir(presetsPath, formatID, presetReg, logger)
				}
			}

			versionedName := formatID + "@" + fmtVersion
			cf := CachedFormat{
				VersionedName: versionedName,
				BaseName:      formatID,
				DisplayName:   cap.DisplayName,
				MimeTypes:     cap.MimeTypes,
				Extensions:    cap.Extensions,
				HasReader:     cap.HasCapability("read"),
				HasWriter:     cap.HasCapability("write"),
				Source:        manifest.Name,
			}
			// Enrich from schema if loaded.
			if s, ok := schemaReg.GetSchema(formatID); ok {
				if cf.DisplayName == "" {
					cf.DisplayName = s.Title
				}
				if len(cf.MimeTypes) == 0 {
					cf.MimeTypes = s.FormatMeta.MimeTypes
				}
				if len(cf.Extensions) == 0 {
					cf.Extensions = s.FormatMeta.Extensions
				}
				cf.FilterClass = s.FormatMeta.Class
			}
			formats = append(formats, cf)

		case "tool":
			// Load tool schema from explicit path if provided.
			if cap.Schema != "" {
				toolID := cap.ID
				if toolID == "" {
					toolID = cap.Name
				}
				schemaPath := filepath.Join(vDir, cap.Schema)
				if err := toolSchemaReg.LoadSchemaFile(schemaPath, toolID); err != nil {
					logf(logger, "loading tool schema %s: %v", schemaPath, err)
				}
			}
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

func collectSchemas(reg *schema.SchemaRegistry) map[string]*schema.FormatSchema {
	ids := reg.FormatIDs()
	if len(ids) == 0 {
		return nil
	}
	result := make(map[string]*schema.FormatSchema, len(ids))
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

// loadPresetsFromDir loads JSON preset files from a directory.
func loadPresetsFromDir(dir string, formatID string, reg *preset.PresetRegistry, logger *log.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logf(logger, "reading presets directory %s: %v", dir, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			logf(logger, "reading preset file %s: %v", entry.Name(), err)
			continue
		}
		var presetData struct {
			ID          string         `json:"id"`
			Name        string         `json:"name"`
			Description string         `json:"description"`
			IsDefault   bool           `json:"isDefault"`
			Parameters  map[string]any `json:"parameters"`
		}
		if err := json.Unmarshal(data, &presetData); err != nil {
			logf(logger, "parsing preset file %s: %v", entry.Name(), err)
			continue
		}
		presetName := presetData.ID
		if presetName == "" {
			presetName = strings.TrimSuffix(entry.Name(), ".json")
		}
		// Strip format prefix: "okf_html-wellFormed" → "wellFormed"
		if strings.HasPrefix(presetName, formatID+"-") {
			presetName = presetName[len(formatID)+1:]
		}
		reg.RegisterFormatPreset(formatID, presetName, &preset.FormatPreset{
			Name:        presetName,
			Description: presetData.Description,
			Format:      formatID,
			Config:      presetData.Parameters,
			Source:      "bridge",
			IsDefault:   presetData.IsDefault,
		})
	}
}

func logf(logger *log.Logger, format string, args ...any) {
	if logger != nil {
		logger.Printf(format, args...)
	}
}
