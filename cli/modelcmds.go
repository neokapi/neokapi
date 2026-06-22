package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// NewModelsCmd builds `kapi models` — manage the large model assets that
// plugins declare in their manifest and that kapi (not the plugin) downloads,
// verifies, and caches in a shared location. See cli/modelassets.go.
func (a *App) NewModelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List and manage the models kapi can translate with",
		Long: "A single view of every model kapi can use, across three sources:\n\n" +
			"  • Local · Ollama  — on-device models served by a local Ollama runtime\n" +
			"                      (`kapi models pull <model>` installs one)\n" +
			"  • Plugin models   — integrity-pinned assets a plugin downloads and caches\n" +
			"                      under $XDG_CACHE_HOME/kapi/models/<plugin>/<id>/<version>/\n" +
			"  • Cloud providers — remote models that require an API key\n\n" +
			"`kapi models pull`/`prune` install and remove Ollama and plugin models; cloud\n" +
			"models are listed for reference. Filter with `--provider <ollama|plugin|cloud-id>`.",
	}
	cmd.AddCommand(a.newModelsListCmd())
	cmd.AddCommand(a.newModelsPullCmd())
	cmd.AddCommand(a.newModelsPruneCmd())
	return cmd
}

// pluginModel pairs a declared model asset with the plugin that declares it.
type pluginModel struct {
	plugin string
	asset  manifest.ModelAsset
}

func (a *App) allPluginModels() []pluginModel {
	var out []pluginModel
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
			out = append(out, pluginModel{plugin: p.Name(), asset: m})
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

// resolveModelRef resolves a user-supplied reference to the (plugin, asset) it
// names. A model id is the primary handle, since ids are globally meaningful and
// the user rarely cares which plugin provides one; the plugin forms only
// disambiguate. In order:
//
//	sat-3l-sm        a model id — kapi finds the plugin that provides it
//	sat/sat-3l-sm    an explicit plugin/model pair (disambiguation)
//	sat              a bare plugin name — its default model
func (a *App) resolveModelRef(ref string) (plugin string, asset manifest.ModelAsset, err error) {
	if p, m, ok := strings.Cut(ref, "/"); ok {
		as, found := a.findModel(p, m)
		if !found {
			return "", manifest.ModelAsset{}, fmt.Errorf("no model %q in plugin %q (see `kapi models list`)", m, p)
		}
		return p, as, nil
	}
	// Bare reference: prefer interpreting it as a model id.
	var matches []pluginModel
	for _, pm := range a.allPluginModels() {
		if pm.asset.ID == ref {
			matches = append(matches, pm)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].plugin, matches[0].asset, nil
	case 0:
		// Not a known model id — maybe it's a plugin name (use its default model).
		if as, ok := a.findModel(ref, ""); ok {
			return ref, as, nil
		}
		return "", manifest.ModelAsset{}, fmt.Errorf("no model or plugin named %q (see `kapi models list`)", ref)
	default:
		where := make([]string, len(matches))
		for i, m := range matches {
			where[i] = m.plugin + "/" + m.asset.ID
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

func (a *App) newModelsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every model kapi can use (Ollama, plugin assets, cloud providers)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter, _ := cmd.Flags().GetString("provider")

			// Ollama models (best-effort: a short timeout so a missing runtime
			// degrades gracefully rather than hanging the listing). Skipped when a
			// filter excludes the Ollama source, so no network call is made.
			var installed []aiprovider.OllamaModelInfo
			ollamaReachable := true
			wantOllama := filter == "" || filter == output.ModelSourceOllama
			if wantOllama {
				ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
				mgr := aiprovider.NewOllamaManager(ollamaBaseURL(cmd))
				models, oerr := mgr.List(ctx)
				cancel()
				ollamaReachable = oerr == nil
				if ollamaReachable {
					installed = models
				}
			}

			rows := buildModelRows(a.allPluginModels(), installed, aiprovider.Providers(), filter)

			if wantOllama && !ollamaReachable {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"note: Ollama not detected — local models show as available to pull; run `kapi ollama status`.")
			}
			return output.Print(cmd, output.ModelsListOutput{Models: rows, Total: len(rows)})
		},
	}
	cmd.Flags().String("provider", "", "Filter to one source/provider (e.g. ollama, anthropic, or a plugin name)")
	return cmd
}

// buildModelRows assembles the unified model view from its three sources. It is
// pure (no network/registry access) so the composition is unit-testable; the
// command supplies live Ollama + plugin + provider data. filter, when non-empty,
// keeps only rows whose source or provider equals it.
func buildModelRows(pluginModels []pluginModel, installed []aiprovider.OllamaModelInfo, providers []aiprovider.ProviderInfo, filter string) []output.ModelRow {
	keep := func(source, provider string) bool {
		return filter == "" || filter == source || filter == provider
	}
	var rows []output.ModelRow

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
				row.Size = humanBytes(mi.Size)
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
				Size:      humanBytes(mi.Size),
			})
		}
	}

	// 2) Plugin models — host-owned assets.
	for _, pm := range pluginModels {
		if !keep(output.ModelSourcePlugin, pm.plugin) {
			continue
		}
		status, bytes := modelStatus(pm.plugin, pm.asset)
		size := ""
		if bytes > 0 {
			size = humanBytes(bytes)
		}
		rows = append(rows, output.ModelRow{
			Source:    output.ModelSourcePlugin,
			Provider:  pm.plugin,
			Model:     pm.asset.ID,
			Version:   pm.asset.Version,
			Default:   pm.asset.Default,
			Status:    status,
			SizeBytes: bytes,
			Size:      size,
		})
	}

	// 3) Cloud providers — remote, need an API key. Listed for reference (their
	//    built-in default model); local/keyless providers are covered above.
	for _, p := range providers {
		if p.Local || p.DefaultModel == "" {
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

func (a *App) newModelsPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <model>",
		Short: "Download a model (Ollama model, or a plugin asset)",
		Long: "Install a model so kapi can translate with it. A bare reference defaults to an\n" +
			"Ollama model (e.g. llama3.2:3b, qwen3:1.7b) and is pulled into the local Ollama\n" +
			"runtime. A plugin model id (e.g. sat-3l-sm), a plugin name, or plugin/model fetches\n" +
			"that plugin's host-owned asset instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			plugin, asset, err := a.resolveModelRef(ref)
			if err != nil {
				// An explicit plugin/model that didn't resolve is an error; a bare
				// unknown reference defaults to an Ollama model.
				if strings.Contains(ref, "/") {
					return err
				}
				return a.pullOllamaModel(cmd, ref)
			}
			if asset.Bundled {
				return fmt.Errorf("%s/%s is bundled with the plugin — nothing to fetch", plugin, asset.ID)
			}
			dir, err := EnsureModel(cmd.Context(), asset, ModelEnsureOptions{
				Plugin: plugin,
				Logf:   func(f string, a ...any) { fmt.Fprintf(cmd.ErrOrStderr(), f+"\n", a...) },
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourcePlugin, Plugin: plugin, Model: asset.ID, Dir: dir, Action: "ready"})
		},
	}
}

// pullOllamaModel installs an Ollama model with streaming progress, failing fast
// with actionable guidance if the runtime is unreachable.
func (a *App) pullOllamaModel(cmd *cobra.Command, name string) error {
	mgr := aiprovider.NewOllamaManager(ollamaBaseURL(cmd))
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
	if err := mgr.Pull(cmd.Context(), name, ollamaPullPrinter(cmd.ErrOrStderr())); err != nil {
		return err
	}
	return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourceOllama, Model: name, Action: "ready"})
}

func (a *App) newModelsPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune <model>",
		Short: "Remove a model (Ollama model, or a cached plugin asset)",
		Long: "Delete a model from disk. A bare reference defaults to an Ollama model; a plugin\n" +
			"model id (e.g. sat-3l-sm), plugin name, or plugin/model removes that cached asset.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			plugin, asset, err := a.resolveModelRef(ref)
			if err != nil {
				if strings.Contains(ref, "/") {
					return err
				}
				return a.pruneOllamaModel(cmd, ref)
			}
			if asset.Bundled {
				return fmt.Errorf("%s/%s is bundled with the plugin — cannot remove", plugin, asset.ID)
			}
			dir, err := ModelDir(plugin, asset.ID, asset.Version)
			if err != nil {
				return err
			}
			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourcePlugin, Plugin: plugin, Model: asset.ID, Action: "absent"})
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("prune %s/%s: %w", plugin, asset.ID, err)
			}
			return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourcePlugin, Plugin: plugin, Model: asset.ID, Dir: dir, Action: "removed"})
		},
	}
}

// pruneOllamaModel removes an installed Ollama model.
func (a *App) pruneOllamaModel(cmd *cobra.Command, name string) error {
	mgr := aiprovider.NewOllamaManager(ollamaBaseURL(cmd))
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
