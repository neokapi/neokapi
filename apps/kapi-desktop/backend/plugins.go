package backend

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/cli/pluginhost"
	pluginhostreg "github.com/neokapi/neokapi/cli/pluginhost/registry"
	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/av"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/core/vision"
)

// formatPluginProviders maps a format the host can't read in-core to the plugin
// that provides it. Used for on-demand install: on macOS the kapi-pdfium plugin
// arrives via the Homebrew Cask → kapi-cli → kapi-pdfium chain, but the
// Linux/Windows desktop ships as a raw artifact with no package manager to
// express that dependency, so the engine has no PDF reader until the plugin is
// installed. Rather than bundle (and duplicate) the engine per platform, we
// fetch it from the registry the first time the user actually opens a PDF.
var formatPluginProviders = map[string]string{"pdf": "pdfium"}

// ensureFormatPlugin installs, once and on demand, the plugin that provides a
// format the host currently can't read (e.g. PDF via kapi-pdfium). It is a
// best-effort, synchronous install so the immediately-following NewReader
// succeeds, and emits the same plugin-installing / plugin-progress /
// plugin-installed / plugin-error events the Plugin Manager uses, so the UI can
// show a toast. A no-op when the reader already exists or no plugin is known.
func (a *App) ensureFormatPlugin(formatName string) {
	if a.formatReg.HasReader(registry.FormatID(formatName)) {
		return
	}
	if plugin, ok := formatPluginProviders[formatName]; ok {
		a.installPluginOnDemand(plugin, formatName)
	}
}

// enginePluginProviders maps a media format to the plugin that provides its
// recognition/demux ENGINE: kapi-vision (image OCR), kapi-asr (audio speech),
// kapi-av (video demux). Unlike formatPluginProviders, these formats already have
// an in-core reader — the file opens as Media without the engine — so the
// on-demand trigger is the engine's availability, not the reader's.
var enginePluginProviders = map[string]string{
	"image": "vision",
	"audio": "asr",
	"video": "av",
}

// mediaEngineAvailable reports whether the local engine that enriches a media
// format is already usable, so ensureMediaEngine can skip a redundant install.
func mediaEngineAvailable(formatName string) bool {
	switch formatName {
	case "image":
		return vision.Available("")
	case "audio":
		return asr.Available("")
	case "video":
		return av.FFmpegAvailable()
	}
	return false
}

// ensureMediaEngine installs, once and on demand, the engine plugin that enriches
// a media format (OCR/ASR/demux) when that engine isn't already available. A
// no-op when the engine is present or the format has no engine provider.
// Best-effort and synchronous, emitting the same plugin-* events as
// ensureFormatPlugin so the UI can show progress.
func (a *App) ensureMediaEngine(formatName string) {
	if mediaEngineAvailable(formatName) {
		return
	}
	if plugin, ok := enginePluginProviders[formatName]; ok {
		a.installPluginOnDemand(plugin, formatName)
	}
}

// installPluginOnDemand fetches a plugin from the registry into the desktop's
// plugin dir, emitting plugin-installing / plugin-progress / plugin-installed /
// plugin-error events, and rescans so a newly installed reader registers. Shared
// by ensureFormatPlugin (reader providers) and ensureMediaEngine (engine
// providers).
func (a *App) installPluginOnDemand(plugin, forWhat string) {
	a.emitEvent("plugin-installing", map[string]string{"name": plugin})
	lastPct := -1
	_, err := pluginhost.InstallFromRegistry(context.Background(), pluginhost.InstallOptions{
		IndexURL:    pluginhost.DefaultIndexURL(),
		PluginName:  plugin,
		KapiVersion: version.Version,
		TargetDir:   a.pluginDir,
		LogF:        func(msg string) { a.logger.Printf("install %s: %s", plugin, msg) },
		ProgressF: func(downloaded, total int64) {
			if total <= 0 {
				return
			}
			if pct := int(downloaded * 100 / total); pct != lastPct {
				lastPct = pct
				a.emitEvent("plugin-progress", map[string]any{"name": plugin, "percent": pct})
			}
		},
	})
	if err != nil {
		a.logger.Printf("on-demand install of %s for %s failed: %v", plugin, forWhat, err)
		a.emitEvent("plugin-error", map[string]string{"name": plugin, "error": err.Error()})
		return
	}
	a.rescanPlugins() // re-registers a newly installed plugin's format reader
	a.emitEvent("plugin-installed", map[string]string{"name": plugin})
	a.emitEvent("plugins-changed", nil)
}

// AvailablePlugin represents a plugin available from the registry.
type AvailablePlugin struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Installed   bool   `json:"installed"`
	// Available is true when the registry has a stable, kapi-compatible build
	// of this plugin for the running OS/arch. When false the UI disables the
	// Install button — e.g. a plugin with no windows/arm64 tarball.
	Available bool `json:"available"`
	// Platform is the running OS/arch ("windows/arm64"), for the UI to explain
	// why an unavailable plugin can't be installed.
	Platform string `json:"platform"`
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
		lastPct := -1
		_, err := pluginhost.InstallFromRegistry(context.Background(), pluginhost.InstallOptions{
			IndexURL:    pluginhost.DefaultIndexURL(),
			PluginName:  name,
			KapiVersion: version.Version,
			TargetDir:   a.pluginDir,
			LogF: func(msg string) {
				a.logger.Printf("install %s: %s", name, msg)
			},
			// Emit download progress, throttled to whole-percent steps so the
			// UI's progress bar advances without flooding the event stream.
			ProgressF: func(downloaded, total int64) {
				if total <= 0 {
					return
				}
				pct := int(downloaded * 100 / total)
				if pct != lastPct {
					lastPct = pct
					a.emitEvent("plugin-progress", map[string]any{"name": name, "percent": pct})
				}
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

// RemovePlugin uninstalls a plugin via the plugin host, which deletes it from
// the directory it was discovered in — the same one InstallPlugin installs into.
//
// The frontend may pass either a plugin name ("okapi-bridge") or the composite
// installation ID surfaced by ListPlugins ("okapi-bridge/1.39.0"). We resolve
// whichever was given to the plugin's declared name before delegating, so the
// uninstall button works regardless of which identifier the UI threads through.
func (a *App) RemovePlugin(idOrName string) error {
	if a.host() == nil {
		return fmt.Errorf("remove %s: plugins not loaded", idOrName)
	}
	name := a.resolvePluginName(idOrName)
	if err := a.host().Remove(name); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	a.rescanPlugins()
	a.emitEvent("plugins-changed", nil)
	a.emitEvent("registries-changed", nil)
	return nil
}

// resolvePluginName maps an installation ID (parentdir/dir, as built by
// ListPlugins) or a plain name to the plugin's declared manifest name. It
// returns the input unchanged when no installed plugin matches, so callers
// still surface a clear "not installed" error from the host.
func (a *App) resolvePluginName(idOrName string) string {
	if a.host() == nil {
		return idOrName
	}
	for _, p := range a.host().Plugins() {
		id := p.Name()
		if p.Dir != "" {
			id = filepath.Base(filepath.Dir(p.Dir)) + "/" + filepath.Base(p.Dir)
		}
		if idOrName == p.Name() || idOrName == id {
			return p.Name()
		}
	}
	return idOrName
}

// CheckPluginUpdates compares installed plugins against the registry
// index. A plugin has an update when the registry's latest version
// (across the plugin's channels) is newer than the installed one.
func (a *App) CheckPluginUpdates() ([]PluginUpdate, error) {
	idx, err := a.fetchIndex()
	if err != nil {
		return nil, fmt.Errorf("check updates: %w", err)
	}
	if a.host() == nil {
		return nil, nil
	}

	var result []PluginUpdate
	for _, p := range a.host().Plugins() {
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
	platform := pluginhostreg.PlatformKey()
	var out []AvailablePlugin
	for name, entry := range idx.Plugins {
		if query != "" && !matchPluginQuery(name, entry.Description, query) {
			continue
		}
		latest := highestVersion(entry)
		// Mirror the install path's resolution (constraint "", channel
		// "stable", this kapi version): if it can't resolve a build for the
		// running platform, the plugin isn't installable here.
		_, _, resErr := idx.Resolve(name, "", "stable", version.Version)
		out = append(out, AvailablePlugin{
			Name:        name,
			Version:     latest,
			Description: entry.Description,
			Type:        "manifest",
			Installed:   installed[name],
			Available:   resErr == nil,
			Platform:    platform,
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
	if a.host() == nil {
		return names
	}
	for _, p := range a.host().Plugins() {
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
	if a.host() == nil {
		return nil
	}
	plugins := a.host().Plugins()
	result := make([]project.InstalledPlugin, 0, len(plugins))
	for _, p := range plugins {
		result = append(result, project.InstalledPlugin{
			Name:    p.Name(),
			Version: p.Manifest.Version,
		})
	}
	return result
}
