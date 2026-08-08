package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/neokapi/neokapi/host/pluginhost"
	pluginreg "github.com/neokapi/neokapi/host/pluginhost/registry"
)

// RegistryPluginEntry is one plugin as the registry search projects it — the
// shared shape behind the CLI's `plugin search` and the desktop's plugin
// browser, so both order results the same way and mark installability the same.
type RegistryPluginEntry struct {
	Name        string
	Version     string
	Description string
	// Installable is true when the registry resolves a build for the running
	// OS/arch (constraint "", channel "stable", this kapi version).
	Installable bool
	// Installed is true when the caller already has this plugin.
	Installed bool
	// Platform is the running OS/arch, for a UI to explain an unavailable build.
	Platform string
}

// SearchRegistry fetches the registry index and projects the plugins whose name
// or description matches query (empty matches all) into RegistryPluginEntry,
// sorted by name so the output is stable — the index is a map, and iterating it
// directly gave the desktop a nondeterministic order. installed marks entries
// the caller already has; refresh forces a cache refresh.
func SearchRegistry(ctx context.Context, indexURL, query string, installed map[string]bool, refresh bool) ([]RegistryPluginEntry, error) {
	if indexURL == "" {
		indexURL = pluginhost.DefaultIndexURL()
	}
	idx, err := pluginreg.FetchOrCached(ctx, indexURL, refresh)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(idx.Plugins))
	for name := range idx.Plugins {
		names = append(names, name)
	}
	sort.Strings(names)

	platform := pluginreg.PlatformKey()
	out := make([]RegistryPluginEntry, 0, len(names))
	for _, name := range names {
		entry := idx.Plugins[name]
		if !pluginreg.MatchQuery(name, entry.Description, query) {
			continue
		}
		// Mirror the install path's resolution (constraint "", channel "stable",
		// this kapi version): if it can't resolve a build for the running
		// platform, the plugin isn't installable here.
		_, _, rerr := idx.Resolve(name, "", "stable", KapiVersion())
		out = append(out, RegistryPluginEntry{
			Name:        name,
			Version:     pluginreg.HighestVersion(entry),
			Description: entry.Description,
			Installable: rerr == nil,
			Installed:   installed[name],
			Platform:    platform,
		})
	}
	return out, nil
}

// InstallPluginFromRegistry refuses a plugin kapi has retired (offline-
// authoritative tombstone) with an actionable error, then installs it. It is
// the single install path, so no surface — the CLI, the desktop plugin manager,
// or an on-demand format/engine install — can install a retired plugin.
func InstallPluginFromRegistry(ctx context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error) {
	if t, ok := pluginhost.LookupTombstone(opts.PluginName); ok {
		return nil, fmt.Errorf("plugin %q was retired in kapi %s and can no longer be installed.\n  Reason: %s.\n  Replacement: %s",
			opts.PluginName, t.RetiredIn, t.Because, t.ReplacementMsg)
	}
	return pluginhost.InstallFromRegistry(ctx, opts)
}

// PluginUpdateOverrides optionally override the channel, constraint and index
// recorded at install time, and carry the install hooks and target dir the
// caller needs.
type PluginUpdateOverrides struct {
	Channel    string
	Constraint string
	IndexURL   string
	Unsafe     bool
	// TargetDir is the install root. Empty uses pluginhost.InstallTarget(); the
	// desktop passes its own isolated plugin dir.
	TargetDir string
	LogF      func(msg string)
	ProgressF func(downloaded, total int64)
}

// UpdatePlugin re-installs an installed plugin using the channel, constraint and
// index URL recorded in installed.json at install time (overrides win) — the
// bookkeeping the desktop's reinstall-with-defaults path silently dropped. It
// returns the install result and the version recorded before the update (empty
// when there is no bookkeeping file).
func UpdatePlugin(ctx context.Context, name string, o PluginUpdateOverrides) (*pluginhost.InstallResult, string, error) {
	base := o.TargetDir
	if base == "" {
		base = pluginhost.InstallTarget()
	}
	meta, err := pluginhost.ReadInstalledMetadata(filepath.Join(base, name))
	if err != nil && !os.IsNotExist(err) {
		return nil, "", err
	}
	if meta == nil {
		meta = &pluginhost.InstalledMetadata{}
	}
	res, err := InstallPluginFromRegistry(ctx, pluginhost.InstallOptions{
		IndexURL:    firstNonEmpty(o.IndexURL, meta.IndexURL),
		PluginName:  name,
		Constraint:  firstNonEmpty(o.Constraint, meta.Constraint),
		Channel:     firstNonEmpty(o.Channel, meta.Channel),
		KapiVersion: KapiVersion(),
		Unsafe:      o.Unsafe,
		TargetDir:   o.TargetDir,
		LogF:        o.LogF,
		ProgressF:   o.ProgressF,
	})
	return res, meta.Version, err
}
