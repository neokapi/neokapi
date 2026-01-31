// Package registry provides plugin discovery and manifest management for
// remote plugin repositories.
package registry

import "strings"

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
