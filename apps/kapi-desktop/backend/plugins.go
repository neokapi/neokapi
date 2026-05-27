package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/cli/pluginhost"
	pluginhostreg "github.com/neokapi/neokapi/cli/pluginhost/registry"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/version"
)

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

// SearchPlugins searches the registry index for plugins whose name or
// description matches the query (substring, case-sensitive).
func (a *App) SearchPlugins(query string) ([]AvailablePlugin, error) {
	idx, err := a.fetchIndex()
	if err != nil {
		return nil, fmt.Errorf("search plugins: %w", err)
	}
	return a.matchPlugins(idx, query), nil
}

// ListAvailablePlugins returns every plugin in the registry index.
func (a *App) ListAvailablePlugins() ([]AvailablePlugin, error) {
	idx, err := a.fetchIndex()
	if err != nil {
		return nil, fmt.Errorf("list available: %w", err)
	}
	return a.matchPlugins(idx, ""), nil
}

// InstallPlugin downloads and installs a plugin asynchronously, emitting
// "plugin-installing" / "plugin-installed" / "plugin-error" events.
func (a *App) InstallPlugin(name string) {
	a.emitEvent("plugin-installing", map[string]string{"name": name})

	go func() {
		_, err := pluginhost.InstallFromRegistry(context.Background(), pluginhost.InstallOptions{
			IndexURL:    pluginhost.DefaultIndexURL(),
			PluginName:  name,
			KapiVersion: version.Version,
			TargetDir:   a.pluginDir,
			LogF: func(msg string) {
				a.logger.Printf("install %s: %s", name, msg)
			},
		})
		if err != nil {
			a.emitEvent("plugin-error", map[string]string{"name": name, "error": err.Error()})
			return
		}

		a.rescanPlugins()
		a.emitEvent("plugin-installed", map[string]string{"name": name})
		a.emitEvent("plugins-changed", nil)
		a.emitEvent("registries-changed", nil)
	}()
}

// UpdatePlugin updates a plugin to the latest version (async).
func (a *App) UpdatePlugin(name string) {
	a.InstallPlugin(name) // install latest overwrites older version
}

// RemovePlugin uninstalls a plugin from the configured plugin directory.
// It removes from a.pluginDir — the SAME directory InstallPlugin installs into
// and rescanPlugins discovers from — not the shared XDG default, which on macOS
// differs from kapiConfigDir()/plugins and would leave the plugin unremovable.
func (a *App) RemovePlugin(name string) error {
	if err := pluginhost.RemoveInstalledFrom(a.pluginDir, name); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	a.rescanPlugins()
	a.emitEvent("plugins-changed", nil)
	a.emitEvent("registries-changed", nil)
	return nil
}

// CheckPluginUpdates compares installed plugins against the registry
// index. A plugin has an update when the registry's latest version
// (across the plugin's channels) is newer than the installed one.
func (a *App) CheckPluginUpdates() ([]PluginUpdate, error) {
	idx, err := a.fetchIndex()
	if err != nil {
		return nil, fmt.Errorf("check updates: %w", err)
	}
	if a.pluginHost == nil {
		return nil, nil
	}

	var result []PluginUpdate
	for _, p := range a.pluginHost.Plugins() {
		entry, ok := idx.Plugins[p.Name()]
		if !ok {
			continue
		}
		latest := highestVersion(entry)
		if latest == "" || latest == p.Manifest.Version {
			continue
		}
		result = append(result, PluginUpdate{
			Name:           p.Name(),
			CurrentVersion: p.Manifest.Version,
			LatestVersion:  latest,
		})
	}
	return result, nil
}

// fetchIndex downloads the registry index, honoring the on-disk cache.
func (a *App) fetchIndex() (*pluginhostreg.IndexV2, error) {
	return pluginhostreg.FetchOrCached(context.Background(), pluginhost.DefaultIndexURL(), false)
}

// matchPlugins flattens the registry index into AvailablePlugin entries,
// filtering by query (empty matches everything) and marking installed
// plugins. Each plugin is represented by its highest-versioned entry.
func (a *App) matchPlugins(idx *pluginhostreg.IndexV2, query string) []AvailablePlugin {
	installed := a.installedNames()
	var out []AvailablePlugin
	for name, entry := range idx.Plugins {
		if query != "" && !matchPluginQuery(name, entry.Description, query) {
			continue
		}
		latest := highestVersion(entry)
		out = append(out, AvailablePlugin{
			Name:        name,
			Version:     latest,
			Description: entry.Description,
			Type:        "manifest",
			Installed:   installed[name],
		})
	}
	return out
}

// matchPluginQuery returns true when query is a substring of either the
// plugin name or its description.
func matchPluginQuery(name, description, query string) bool {
	return strings.Contains(name, query) || strings.Contains(description, query)
}

// highestVersion returns the lexicographically-highest version key in
// entry.Versions. Registry entries are short enough that lexicographic
// ordering matches semver in practice; callers needing strict semver
// ordering can lean on pluginhost/registry.Resolve which the install
// path already does.
func highestVersion(entry pluginhostreg.PluginEntry) string {
	var best string
	for v := range entry.Versions {
		if v > best {
			best = v
		}
	}
	return best
}

func (a *App) installedNames() map[string]bool {
	names := make(map[string]bool)
	if a.pluginHost == nil {
		return names
	}
	for _, p := range a.pluginHost.Plugins() {
		names[p.Name()] = true
	}
	return names
}

// --- Plugin status checking ---

// CheckProjectPlugins checks whether a project's declared plugin requirements
// are satisfied by the currently installed plugins. Delegates to the shared
// project.CheckPlugins implementation in core/project.
func (a *App) CheckProjectPlugins(tabID string) *project.PluginStatus {
	op := a.getOpenProject(tabID)
	if op == nil {
		return &project.PluginStatus{Satisfied: true}
	}
	return project.CheckPlugins(op.Project, a.installedPluginList())
}

// installedPluginList returns installed plugins as project.InstalledPlugin values.
func (a *App) installedPluginList() []project.InstalledPlugin {
	if a.pluginHost == nil {
		return nil
	}
	plugins := a.pluginHost.Plugins()
	result := make([]project.InstalledPlugin, 0, len(plugins))
	for _, p := range plugins {
		result = append(result, project.InstalledPlugin{
			Name:    p.Name(),
			Version: p.Manifest.Version,
		})
	}
	return result
}
