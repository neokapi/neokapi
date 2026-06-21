package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// NewModelsCmd builds `kapi models` — manage the large model assets that
// plugins declare in their manifest and that kapi (not the plugin) downloads,
// verifies, and caches in a shared location. See cli/modelassets.go.
func (a *App) NewModelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "Manage model assets that plugins use",
		Long: "List, pre-fetch, and remove the model assets that kapi downloads and caches on a\n" +
			"plugin's behalf. Each is integrity-pinned in the plugin's manifest and stored under\n" +
			"$XDG_CACHE_HOME/kapi/models/<plugin>/<id>/<version>/.",
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
//	gemma-4-e2b      a model id — kapi finds the plugin that provides it
//	llm/gemma-4-e2b  an explicit plugin/model pair (disambiguation)
//	llm              a bare plugin name — its default model
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
	return &cobra.Command{
		Use:   "list",
		Short: "List model assets declared by installed plugins",
		RunE: func(cmd *cobra.Command, _ []string) error {
			models := a.allPluginModels()
			rows := make([]output.ModelAssetRow, 0, len(models))
			for _, pm := range models {
				status, bytes := modelStatus(pm.plugin, pm.asset)
				// A bundled model has no download size, and a downloadable one
				// may omit it; show "—" rather than a misleading 0B.
				size := "—"
				if bytes > 0 {
					size = humanBytes(bytes)
				}
				rows = append(rows, output.ModelAssetRow{
					Plugin:    pm.plugin,
					Model:     pm.asset.ID,
					Version:   pm.asset.Version,
					Default:   pm.asset.Default,
					Status:    status,
					SizeBytes: bytes,
					Size:      size,
				})
			}
			return output.Print(cmd, output.ModelsListOutput{Models: rows, Total: len(rows)})
		},
	}
}

func (a *App) newModelsPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <model>",
		Short: "Download and cache a model asset",
		Long: "Fetch and verify a model asset ahead of time so its first use is instant.\n" +
			"<model> is a model id (e.g. gemma-4-e2b); kapi finds the plugin that provides\n" +
			"it. You may also pass a plugin name (its default model) or plugin/model.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plugin, asset, err := a.resolveModelRef(args[0])
			if err != nil {
				return err
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
			return output.Print(cmd, output.ModelActionOutput{Plugin: plugin, Model: asset.ID, Dir: dir, Action: "ready"})
		},
	}
}

func (a *App) newModelsPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune <model>",
		Short: "Remove a cached model asset from disk",
		Long: "Delete a cached model asset. <model> is a model id (e.g. gemma-4-e2b); a plugin\n" +
			"name or plugin/model also work.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plugin, asset, err := a.resolveModelRef(args[0])
			if err != nil {
				return err
			}
			if asset.Bundled {
				return fmt.Errorf("%s/%s is bundled with the plugin — cannot remove", plugin, asset.ID)
			}
			dir, err := ModelDir(plugin, asset.ID, asset.Version)
			if err != nil {
				return err
			}
			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				return output.Print(cmd, output.ModelActionOutput{Plugin: plugin, Model: asset.ID, Action: "absent"})
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("prune %s/%s: %w", plugin, asset.ID, err)
			}
			return output.Print(cmd, output.ModelActionOutput{Plugin: plugin, Model: asset.ID, Dir: dir, Action: "removed"})
		},
	}
}
