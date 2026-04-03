package backend

import (
	"fmt"

	"github.com/neokapi/neokapi/core/plugin/registry"
)

const defaultRegistryURL = "https://neokapi.github.io/registry/plugins.json"

// Capability describes a specific format or tool provided by a plugin.
type Capability struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Description string   `json:"description,omitempty"`
	MimeTypes   []string `json:"mime_types,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
}

// PluginSearchResult describes a plugin available in the registry.
type PluginSearchResult struct {
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	PluginType   string       `json:"plugin_type"`
	InstallType  string       `json:"install_type"`
	Capabilities []Capability `json:"capabilities,omitempty"`
}

// PluginInstallResult describes the outcome of a plugin install or update.
type PluginInstallResult struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	InstallType string   `json:"install_type"`
	Files       []string `json:"files"`
}

// PluginUpdateInfo describes an available update for an installed plugin.
type PluginUpdateInfo struct {
	Name             string `json:"name"`
	InstalledVersion string `json:"installed_version"`
	AvailableVersion string `json:"available_version"`
}

// remoteRegistry returns a RemoteRegistry using the app's configuration.
// The pluginSearchRegistry field allows tests to override the registry URL.
func (a *App) remoteRegistry() *registry.RemoteRegistry {
	url := a.pluginSearchRegistry
	if url == "" {
		url = defaultRegistryURL
	}
	return registry.NewRemoteRegistry(url, a.PluginDir())
}

// SearchPlugins searches the remote registry for plugins matching the query.
func (a *App) SearchPlugins(query string) ([]PluginSearchResult, error) {
	reg := a.remoteRegistry()
	manifests, err := reg.SearchPlugins(query)
	if err != nil {
		return nil, fmt.Errorf("searching plugins: %w", err)
	}
	return manifestsToSearchResults(manifests), nil
}

// ListAvailablePlugins returns all plugins available in the registry.
func (a *App) ListAvailablePlugins() ([]PluginSearchResult, error) {
	reg := a.remoteRegistry()
	manifests, err := reg.ListAvailable()
	if err != nil {
		return nil, fmt.Errorf("listing available plugins: %w", err)
	}
	return manifestsToSearchResults(manifests), nil
}

// InstallPlugin downloads and installs a plugin by name from the registry.
func (a *App) InstallPlugin(name string) (*PluginInstallResult, error) {
	reg := a.remoteRegistry()
	result, err := reg.InstallPlugin(registry.ParsePluginRef(name))
	if err != nil {
		return nil, fmt.Errorf("installing plugin %s: %w", name, err)
	}
	return &PluginInstallResult{
		Name:        result.Name,
		Version:     result.Version,
		InstallType: result.InstallType,
		Files:       result.Files,
	}, nil
}

// CheckPluginUpdates compares installed plugins against the registry.
func (a *App) CheckPluginUpdates() ([]PluginUpdateInfo, error) {
	reg := a.remoteRegistry()
	updates, err := reg.CheckUpdates()
	if err != nil {
		return nil, fmt.Errorf("checking updates: %w", err)
	}
	out := make([]PluginUpdateInfo, len(updates))
	for i, u := range updates {
		out[i] = PluginUpdateInfo{
			Name:             u.Name,
			InstalledVersion: u.InstalledVersion,
			AvailableVersion: u.AvailableVersion,
		}
	}
	return out, nil
}

// UpdatePlugin updates a specific plugin to the latest registry version.
func (a *App) UpdatePlugin(name string) (*PluginInstallResult, error) {
	return a.InstallPlugin(name)
}

// SearchPluginsByMimeType searches the registry for plugins that handle the given MIME type.
func (a *App) SearchPluginsByMimeType(mimeType string) ([]PluginSearchResult, error) {
	reg := a.remoteRegistry()
	manifests, err := reg.SearchPluginsAdvanced(registry.SearchOptions{MimeType: mimeType})
	if err != nil {
		return nil, fmt.Errorf("searching plugins by MIME type: %w", err)
	}
	return manifestsToSearchResults(manifests), nil
}

// SearchPluginsByType searches the registry for plugins with the given capability type.
func (a *App) SearchPluginsByType(capType string) ([]PluginSearchResult, error) {
	reg := a.remoteRegistry()
	manifests, err := reg.SearchPluginsAdvanced(registry.SearchOptions{Type: capType})
	if err != nil {
		return nil, fmt.Errorf("searching plugins by type: %w", err)
	}
	return manifestsToSearchResults(manifests), nil
}

func manifestsToSearchResults(manifests []registry.PluginManifest) []PluginSearchResult {
	results := make([]PluginSearchResult, len(manifests))
	for i, m := range manifests {
		installType := m.InstallType
		if installType == "" {
			installType = "binary"
		}
		var caps []Capability
		for _, c := range m.Capabilities {
			caps = append(caps, Capability{
				Type:        c.Type,
				Name:        c.Name,
				DisplayName: c.DisplayName,
				Description: c.Description,
				MimeTypes:   c.MimeTypes,
				Extensions:  c.Extensions,
			})
		}
		results[i] = PluginSearchResult{
			Name:         m.Name,
			Version:      m.Version,
			Description:  m.Description,
			PluginType:   m.PluginType,
			InstallType:  installType,
			Capabilities: caps,
		}
	}
	return results
}
