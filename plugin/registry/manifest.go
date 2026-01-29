// Package registry provides plugin discovery and manifest management for
// remote plugin repositories.
package registry

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

	// Platform is the target platform in GOOS/GOARCH format (e.g., "linux/amd64").
	Platform string `json:"platform"`

	// Checksum is the SHA-256 hex digest of the plugin binary.
	Checksum string `json:"checksum"`

	// DownloadURL is the URL to download the plugin binary.
	DownloadURL string `json:"download_url"`

	// MinHostVersion is the minimum gokapi version required to run this plugin.
	MinHostVersion string `json:"min_host_version,omitempty"`
}

// RegistryIndex is the top-level structure returned by a remote plugin registry.
type RegistryIndex struct {
	// Plugins lists all available plugin manifests.
	Plugins []PluginManifest `json:"plugins"`
}
