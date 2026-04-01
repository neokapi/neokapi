// Package cache provides a pre-computed plugin cache that eliminates
// directory scanning, manifest reading, and schema parsing at runtime.
//
// The cache is built at plugin install/update/remove time and stored as
// a single JSON file ({plugin_dir}/plugin-cache.json). At startup, kapi
// reads this one file instead of walking the plugin directory tree.
//
// If the cache is missing or stale, a full scan is performed and the
// cache is rebuilt transparently.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/plugin/registry"
	"github.com/neokapi/neokapi/core/preset"
)

// CacheFileName is the name of the cache file within the plugin directory.
const CacheFileName = "plugin-cache.json"

// CacheVersion is the schema version. Increment when the cache structure changes.
// v6: Added Title field to PropertySchema (property titles from Okapi schemas).
const CacheVersion = 6

// PluginCache is the top-level cache structure written to plugin-cache.json.
// It contains all metadata needed to populate format/schema/preset registries
// at startup without any directory scanning or file parsing.
type PluginCache struct {
	Version     int                             `json:"version"`
	GeneratedAt string                          `json:"generated_at"`
	Plugins     []CachedPlugin                  `json:"plugins"`
	Schemas     map[string]*schema.FormatSchema `json:"schemas,omitempty"`
	ToolSchemas map[string]*schema.FormatSchema `json:"tool_schemas,omitempty"`
	Presets     CachedPresets                   `json:"presets"`
	DocsDir     string                          `json:"docs_dir,omitempty"` // path to docs/ directory (if available)
}

// CachedPlugin holds all metadata for a single installed plugin version.
type CachedPlugin struct {
	Name             string                    `json:"name"`
	Version          string                    `json:"version"`
	FrameworkVersion string                    `json:"framework_version,omitempty"`
	InstallType      string                    `json:"install_type"`
	PluginType       string                    `json:"plugin_type,omitempty"`
	Dir              string                    `json:"dir"`
	Formats          []CachedFormat            `json:"formats,omitempty"`
	Manifest         *registry.BundledManifest `json:"manifest,omitempty"`
}

// CachedFormat describes a single format provided by a plugin.
type CachedFormat struct {
	VersionedName string   `json:"versioned_name"`
	BaseName      string   `json:"base_name"`
	DisplayName   string   `json:"display_name,omitempty"`
	FilterClass   string   `json:"filter_class,omitempty"`
	MimeTypes     []string `json:"mime_types,omitempty"`
	Extensions    []string `json:"extensions,omitempty"`
	HasReader     bool     `json:"has_reader"`
	HasWriter     bool     `json:"has_writer"`
	Source        string   `json:"source"`
}

// CachedPresets stores all presets collected from plugins.
type CachedPresets struct {
	FormatPresets    map[string]map[string]*preset.FormatPreset `json:"format_presets,omitempty"`
	FrameworkPresets map[string]*preset.FrameworkPreset         `json:"framework_presets,omitempty"`
}

// CachePath returns the path to the cache file for the given plugin directory.
func CachePath(pluginDir string) string {
	return filepath.Join(pluginDir, CacheFileName)
}

// Read loads a PluginCache from the cache file in the given plugin directory.
// Returns an error if the file doesn't exist, is corrupt, or has an
// incompatible version.
func Read(pluginDir string) (*PluginCache, error) {
	path := CachePath(pluginDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading plugin cache: %w", err)
	}
	var c PluginCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing plugin cache: %w", err)
	}
	if c.Version != CacheVersion {
		return nil, fmt.Errorf("plugin cache version %d != expected %d", c.Version, CacheVersion)
	}
	return &c, nil
}

// Write serializes a PluginCache to the cache file in the given plugin directory.
func Write(pluginDir string, c *PluginCache) error {
	c.Version = CacheVersion
	c.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling plugin cache: %w", err)
	}

	path := CachePath(pluginDir)

	// Atomic write: write to temp file then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing plugin cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming plugin cache: %w", err)
	}
	return nil
}
