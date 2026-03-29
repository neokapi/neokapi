package backend

import (
	"fmt"

	pluginreg "github.com/neokapi/neokapi/core/plugin/registry"
)

const defaultRegistryURL = "https://neokapi.github.io/registry/plugins.json"

// AvailablePlugin represents a plugin available from the registry.
type AvailablePlugin struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Installed   bool   `json:"installed"`
}

// PluginUpdate represents a plugin with an available update.
type PluginUpdate struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
}

// SearchPlugins searches the remote registry for plugins matching the query.
func (a *App) SearchPlugins(query string) ([]AvailablePlugin, error) {
	reg := a.remoteRegistry()
	if reg == nil {
		return nil, fmt.Errorf("plugin registry not configured")
	}

	results, err := reg.SearchPlugins(query)
	if err != nil {
		return nil, fmt.Errorf("search plugins: %w", err)
	}

	installed := a.installedNames()
	var plugins []AvailablePlugin
	for _, m := range results {
		plugins = append(plugins, AvailablePlugin{
			Name:        m.Name,
			Version:     m.Version,
			Description: m.Description,
			Type:        m.InstallType,
			Installed:   installed[m.Name],
		})
	}
	return plugins, nil
}

// ListAvailablePlugins returns all plugins from the remote registry.
func (a *App) ListAvailablePlugins() ([]AvailablePlugin, error) {
	reg := a.remoteRegistry()
	if reg == nil {
		return nil, fmt.Errorf("plugin registry not configured")
	}

	groups, err := reg.ListAvailableGrouped()
	if err != nil {
		return nil, fmt.Errorf("list available: %w", err)
	}

	installed := a.installedNames()
	var plugins []AvailablePlugin
	for _, g := range groups {
		plugins = append(plugins, AvailablePlugin{
			Name:        g.Name,
			Version:     g.Latest.Version,
			Description: g.Latest.Description,
			Type:        g.Latest.InstallType,
			Installed:   installed[g.Name],
		})
	}
	return plugins, nil
}

// InstallPlugin installs a plugin by name from the remote registry.
// Runs in the background and emits "plugin-installed" or "plugin-error" events.
func (a *App) InstallPlugin(name string) {
	reg := a.remoteRegistry()
	if reg == nil {
		a.emitEvent("plugin-error", map[string]string{"name": name, "error": "plugin registry not configured"})
		return
	}

	a.emitEvent("plugin-installing", map[string]string{"name": name})

	go func() {
		ref := pluginreg.ParsePluginRef(name)
		_, err := reg.InstallPlugin(ref)
		if err != nil {
			a.emitEvent("plugin-error", map[string]string{"name": name, "error": err.Error()})
			return
		}

		if scanErr := a.pluginLoader.ScanMetadata(); scanErr != nil {
			a.logger.Printf("re-scan after install: %v", scanErr)
		}
		a.emitEvent("plugin-installed", map[string]string{"name": name})
		a.emitEvent("plugins-changed", nil)
	}()
}

// UpdatePlugin updates a plugin to the latest version (async).
func (a *App) UpdatePlugin(name string) {
	a.InstallPlugin(name) // install latest overwrites older version
}

// RemovePlugin uninstalls a plugin.
func (a *App) RemovePlugin(name string) error {
	reg := a.remoteRegistry()
	if reg == nil {
		return fmt.Errorf("plugin registry not configured")
	}

	ref := pluginreg.ParsePluginRef(name)
	if err := reg.RemovePlugin(ref); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}

	if scanErr := a.pluginLoader.ScanMetadata(); scanErr != nil {
		a.logger.Printf("re-scan after remove: %v", scanErr)
	}
	a.emitEvent("plugins-changed", nil)
	return nil
}

// CheckPluginUpdates checks for available updates.
func (a *App) CheckPluginUpdates() ([]PluginUpdate, error) {
	reg := a.remoteRegistry()
	if reg == nil {
		return nil, fmt.Errorf("plugin registry not configured")
	}

	updates, err := reg.CheckUpdates()
	if err != nil {
		return nil, fmt.Errorf("check updates: %w", err)
	}

	var result []PluginUpdate
	for _, u := range updates {
		result = append(result, PluginUpdate{
			Name:           u.Name,
			CurrentVersion: u.InstalledVersion,
			LatestVersion:  u.AvailableVersion,
		})
	}
	return result, nil
}

func (a *App) remoteRegistry() *pluginreg.RemoteRegistry {
	a.registryMu.Lock()
	defer a.registryMu.Unlock()
	if a.registry != nil {
		return a.registry
	}
	dir := a.pluginLoader.Dir()
	if dir == "" {
		return nil
	}
	a.registry = pluginreg.NewRemoteRegistry(defaultRegistryURL, dir)
	return a.registry
}

func (a *App) installedNames() map[string]bool {
	names := make(map[string]bool)
	for _, p := range a.pluginLoader.Plugins() {
		names[p.Name] = true
	}
	return names
}
