package host

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// OrNone renders an empty config value as "(none)" for display.
func OrNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// PluginModel pairs a declared model asset with the plugin that declares it.
type PluginModel struct {
	Plugin string
	Asset  manifest.ModelAsset
}

func (a *App) AllPluginModels() []PluginModel {
	var out []PluginModel
	if a.PluginHost == nil {
		return out
	}
	for _, p := range a.PluginHost.Plugins() {
		if p.Manifest == nil {
			continue
		}
		// A retired plugin is inert — it contributes no models to the view.
		if p.Retired != nil {
			continue
		}
		for _, m := range p.Manifest.Models {
			out = append(out, PluginModel{Plugin: p.Name(), Asset: m})
		}
	}
	return out
}

// findModel resolves a plugin name + optional model id to its declared asset.
// An empty model id resolves to the plugin's default model.
func (a *App) findModel(plugin, modelID string) (manifest.ModelAsset, bool) {
	if a.PluginHost == nil {
		return manifest.ModelAsset{}, false
	}
	p := a.PluginHost.Plugin(plugin)
	if p == nil || p.Manifest == nil {
		return manifest.ModelAsset{}, false
	}
	if modelID == "" {
		return p.Manifest.DefaultModel()
	}
	return p.Manifest.Model(modelID)
}

// ResolveModelRef resolves a user-supplied reference to the (plugin, asset) it
// names. A model id is the primary handle, since ids are globally meaningful and
// the user rarely cares which plugin provides one; the plugin forms only
// disambiguate. In order:
//
//	sat-3l-sm        a model id — kapi finds the plugin that provides it
//	sat/sat-3l-sm    an explicit plugin/model pair (disambiguation)
//	sat              a bare plugin name — its default model
func (a *App) ResolveModelRef(ref string) (plugin string, asset manifest.ModelAsset, err error) {
	if p, m, ok := strings.Cut(ref, "/"); ok {
		as, found := a.findModel(p, m)
		if !found {
			return "", manifest.ModelAsset{}, fmt.Errorf("no model %q in plugin %q (see `kapi models list`)", m, p)
		}
		return p, as, nil
	}
	// Bare reference: prefer interpreting it as a model id.
	var matches []PluginModel
	for _, pm := range a.AllPluginModels() {
		if pm.Asset.ID == ref {
			matches = append(matches, pm)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].Plugin, matches[0].Asset, nil
	case 0:
		// Not a known model id — maybe it's a plugin name (use its default model).
		if as, ok := a.findModel(ref, ""); ok {
			return ref, as, nil
		}
		return "", manifest.ModelAsset{}, fmt.Errorf("no model or plugin named %q (see `kapi models list`)", ref)
	default:
		where := make([]string, len(matches))
		for i, m := range matches {
			where[i] = m.Plugin + "/" + m.Asset.ID
		}
		return "", manifest.ModelAsset{}, fmt.Errorf("model id %q is provided by multiple plugins (%s); qualify it as plugin/model", ref, strings.Join(where, ", "))
	}
}

// modelStatus reports whether every file of an asset is present in its cache
// dir, plus its total declared size in bytes.
func modelStatus(plugin string, asset manifest.ModelAsset) (status string, totalBytes int64) {
	for _, f := range asset.Files {
		totalBytes += f.Size
	}
	if asset.Bundled {
		return "bundled", totalBytes // ships in the tarball; nothing to fetch
	}
	dir, err := ModelDir(plugin, asset.ID, asset.Version)
	if err != nil {
		return "unknown", totalBytes
	}
	for _, f := range asset.Files {
		if !modelFilePresent(filepath.Join(dir, f.Path), f.Size) {
			return "not cached", totalBytes
		}
	}
	return "cached", totalBytes
}

// BuildModelRows assembles the unified model view from its three sources. It is
// pure (no network/registry access) so the composition is unit-testable; the
// command supplies live Ollama + plugin + provider data. filter, when non-empty,
// keeps only rows whose source or provider equals it.
func BuildModelRows(pluginModels []PluginModel, installed []aiprovider.OllamaModelInfo, providers []aiprovider.ProviderInfo, claudeCodeDetected bool, filter string) []output.ModelRow {
	keep := func(source, provider string) bool {
		return filter == "" || filter == source || filter == provider
	}
	var rows []output.ModelRow

	// 0) Detected — keyless providers present on this machine. Claude Code is
	//    listed on binary presence alone (auth is verified lazily at first call,
	//    with actionable errors), so the listing stays instant and offline.
	if claudeCodeDetected && keep(output.ModelSourceDetected, string(aiprovider.ClaudeCode)) {
		rows = append(rows, output.ModelRow{
			Source:   output.ModelSourceDetected,
			Provider: string(aiprovider.ClaudeCode),
			Model:    aiprovider.DefaultClaudeCodeModel,
			Status:   "detected",
			Note:     "uses your Claude subscription",
		})
	}

	// 1) Local · Ollama — recommended picks first (marking which are installed),
	//    then any other installed models.
	installedByName := make(map[string]aiprovider.OllamaModelInfo, len(installed))
	for _, mi := range installed {
		installedByName[mi.Name] = mi
	}
	lookup := func(name string) (aiprovider.OllamaModelInfo, bool) {
		if mi, ok := installedByName[name]; ok {
			return mi, true
		}
		if !strings.Contains(name, ":") {
			mi, ok := installedByName[name+":latest"]
			return mi, ok
		}
		return aiprovider.OllamaModelInfo{}, false
	}
	seen := make(map[string]bool)
	if keep(output.ModelSourceOllama, output.ModelSourceOllama) {
		for _, rec := range aiprovider.RecommendedOllamaModels {
			row := output.ModelRow{
				Source:   output.ModelSourceOllama,
				Provider: output.ModelSourceOllama,
				Model:    rec.Name,
				Note:     rec.Note,
				Default:  rec.Name == aiprovider.DefaultOllamaModel,
				Status:   "available",
			}
			if mi, ok := lookup(rec.Name); ok {
				row.Status = "installed"
				row.SizeBytes = mi.Size
				row.Size = HumanBytes(mi.Size)
			}
			rows = append(rows, row)
			seen[rec.Name] = true
		}
		for _, mi := range installed {
			if seen[mi.Name] {
				continue
			}
			rows = append(rows, output.ModelRow{
				Source:    output.ModelSourceOllama,
				Provider:  output.ModelSourceOllama,
				Model:     mi.Name,
				Status:    "installed",
				SizeBytes: mi.Size,
				Size:      HumanBytes(mi.Size),
			})
		}
	}

	// 2) Plugin models — host-owned assets.
	for _, pm := range pluginModels {
		if !keep(output.ModelSourcePlugin, pm.Plugin) {
			continue
		}
		status, bytes := modelStatus(pm.Plugin, pm.Asset)
		size := ""
		if bytes > 0 {
			size = HumanBytes(bytes)
		}
		rows = append(rows, output.ModelRow{
			Source:    output.ModelSourcePlugin,
			Provider:  pm.Plugin,
			Model:     pm.Asset.ID,
			Version:   pm.Asset.Version,
			Default:   pm.Asset.Default,
			Status:    status,
			SizeBytes: bytes,
			Size:      size,
		})
	}

	// 3) Cloud providers — remote, need an API key. Listed for reference (their
	//    built-in default model); local/keyless providers are covered above.
	for _, p := range providers {
		if !p.NeedsKey() || p.DefaultModel == "" {
			continue
		}
		if !keep(output.ModelSourceCloud, string(p.Name)) {
			continue
		}
		rows = append(rows, output.ModelRow{
			Source:   output.ModelSourceCloud,
			Provider: string(p.Name),
			Model:    p.DefaultModel,
			Status:   "cloud · needs key",
		})
	}

	return rows
}

// PullOllamaModel installs an Ollama model with streaming progress, failing fast
// with actionable guidance if the runtime is unreachable.
func (a *App) PullOllamaModel(cmd Command, name string) error {
	mgr := aiprovider.NewOllamaManager(OllamaBaseURL(cmd))
	if _, err := mgr.Version(cmd.Context()); err != nil {
		return err
	}
	has, err := mgr.Has(cmd.Context(), name)
	if err != nil {
		return err
	}
	if has {
		return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourceOllama, Model: name, Action: "present"})
	}
	if err := mgr.Pull(cmd.Context(), name, OllamaPullPrinter(cmd.ErrOrStderr())); err != nil {
		return err
	}
	return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourceOllama, Model: name, Action: "ready"})
}

// PruneOllamaModel removes an installed Ollama model.
func (a *App) PruneOllamaModel(cmd Command, name string) error {
	mgr := aiprovider.NewOllamaManager(OllamaBaseURL(cmd))
	if _, err := mgr.Version(cmd.Context()); err != nil {
		return err
	}
	has, err := mgr.Has(cmd.Context(), name)
	if err != nil {
		return err
	}
	if !has {
		return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourceOllama, Model: name, Action: "absent"})
	}
	if err := mgr.Delete(cmd.Context(), name); err != nil {
		return err
	}
	return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourceOllama, Model: name, Action: "removed"})
}
