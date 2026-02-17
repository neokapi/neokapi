// Package registry provides plugin discovery and manifest management for
// remote plugin repositories.
package registry

import (
	"fmt"
	"sort"
	"strings"
)

// Capability describes a specific format or tool provided by a plugin.
type Capability struct {
	// Type is "format" or "tool".
	Type string `json:"type"`

	// Name is the capability identifier (e.g., "openxml", "html").
	Name string `json:"name"`

	// DisplayName is a human-readable name (e.g., "Microsoft Office (OpenXML)").
	DisplayName string `json:"display_name,omitempty"`

	// Description is a short description of the capability.
	Description string `json:"description,omitempty"`

	// MimeTypes lists MIME types handled by this capability.
	MimeTypes []string `json:"mime_types,omitempty"`

	// Extensions lists file extensions handled by this capability (e.g., ".docx").
	Extensions []string `json:"extensions,omitempty"`
}

// PluginManifest describes a plugin available for download from a remote registry.
type PluginManifest struct {
	// Name is the unique identifier for the plugin (e.g., "csv-reader").
	Name string `json:"name"`

	// Version is the semantic version of the plugin (e.g., "1.2.0").
	Version string `json:"version"`

	// Description is a human-readable description of the plugin.
	Description string `json:"description"`

	// PluginType is the type of plugin: "format-reader", "format-writer", or "tool".
	PluginType string `json:"plugin_type"`

	// InstallType controls how the plugin is installed: "binary" (default) or "bridge".
	// Binary plugins are standalone executables. Bridge plugins are tar.gz archives
	// containing a JAR and a .bridge.json descriptor.
	InstallType string `json:"install_type,omitempty"`

	// Platform is the target platform in GOOS/GOARCH format (e.g., "linux/amd64").
	Platform string `json:"platform"`

	// Checksum is the SHA-256 hex digest of the plugin binary.
	Checksum string `json:"checksum"`

	// DownloadURL is the URL to download the plugin binary.
	DownloadURL string `json:"download_url"`

	// MinHostVersion is the minimum gokapi version required to run this plugin.
	MinHostVersion string `json:"min_host_version,omitempty"`

	// Capabilities lists the fine-grained capabilities this plugin provides.
	Capabilities []Capability `json:"capabilities,omitempty"`
}

// HasMimeType reports whether any capability in this manifest handles the given MIME type.
func (m *PluginManifest) HasMimeType(mimeType string) bool {
	mt := strings.ToLower(mimeType)
	for _, cap := range m.Capabilities {
		for _, cm := range cap.MimeTypes {
			if strings.ToLower(cm) == mt {
				return true
			}
		}
	}
	return false
}

// HasCapabilityType reports whether this manifest has any capability of the given type
// (e.g., "format" or "tool").
func (m *PluginManifest) HasCapabilityType(capType string) bool {
	ct := strings.ToLower(capType)
	for _, cap := range m.Capabilities {
		if strings.ToLower(cap.Type) == ct {
			return true
		}
	}
	return false
}

// RegistryIndex is the top-level structure returned by a remote plugin registry.
type RegistryIndex struct {
	// Version is the schema version of the registry index (currently 1).
	Version int `json:"version"`

	// Plugins lists all available plugin manifests.
	Plugins []PluginManifest `json:"plugins"`
}

// PluginGroup holds all available versions of a plugin grouped by name.
type PluginGroup struct {
	// Name is the plugin name (e.g., "okapi").
	Name string

	// Latest is the manifest with the highest version.
	Latest PluginManifest

	// Versions contains all versions sorted descending by semantic version.
	Versions []PluginManifest
}

// GroupByName groups manifests by plugin name for the given platform,
// returning groups sorted alphabetically with versions sorted descending.
func (idx *RegistryIndex) GroupByName(platform string) []PluginGroup {
	byName := make(map[string][]PluginManifest)
	for _, m := range idx.Plugins {
		if m.Platform != platform && m.Platform != "" {
			continue
		}
		byName[m.Name] = append(byName[m.Name], m)
	}

	groups := make([]PluginGroup, 0, len(byName))
	for name, manifests := range byName {
		sort.Slice(manifests, func(i, j int) bool {
			return CompareSemver(manifests[i].Version, manifests[j].Version) > 0
		})
		groups = append(groups, PluginGroup{
			Name:     name,
			Latest:   manifests[0],
			Versions: manifests,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})

	return groups
}

// FindVersions returns all manifests for the given plugin name and platform,
// sorted with the latest version first.
func (idx *RegistryIndex) FindVersions(name, platform string) []PluginManifest {
	var matches []PluginManifest
	for _, m := range idx.Plugins {
		if m.Name == name && (m.Platform == platform || m.Platform == "") {
			matches = append(matches, m)
		}
	}
	// Sort by version descending.
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if CompareSemver(matches[j].Version, matches[i].Version) > 0 {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
	return matches
}

// FindLatest returns the manifest with the highest version for the given
// plugin name and platform.
func (idx *RegistryIndex) FindLatest(name, platform string) (*PluginManifest, error) {
	versions := idx.FindVersions(name, platform)
	if len(versions) == 0 {
		return nil, fmt.Errorf("plugin %q not found for platform %s", name, platform)
	}
	return &versions[0], nil
}

// FindExactVersion returns the manifest matching the given name, version,
// and platform exactly.
func (idx *RegistryIndex) FindExactVersion(name, version, platform string) (*PluginManifest, error) {
	for _, m := range idx.Plugins {
		if m.Name == name && m.Version == version && (m.Platform == platform || m.Platform == "") {
			return &m, nil
		}
	}
	return nil, fmt.Errorf("plugin %q version %s not found for platform %s", name, version, platform)
}
