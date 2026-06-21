package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

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

// splitModelRef parses "plugin/model" (or bare "plugin") into its parts.
func splitModelRef(s string) (plugin, model string) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// modelStatus reports whether every file of an asset is present in its cache
// dir, plus its total declared size.
func modelStatus(plugin string, asset manifest.ModelAsset) (status, size string) {
	var total int64
	for _, f := range asset.Files {
		total += f.Size
	}
	size = humanBytes(total)
	dir, err := ModelDir(plugin, asset.ID, asset.Version)
	if err != nil {
		return "unknown", size
	}
	for _, f := range asset.Files {
		if !modelFilePresent(filepath.Join(dir, f.Path), f.Size) {
			return "not cached", size
		}
	}
	return "cached", size
}

func (a *App) newModelsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List model assets declared by installed plugins",
		RunE: func(cmd *cobra.Command, _ []string) error {
			models := a.allPluginModels()
			w := cmd.OutOrStdout()
			if len(models) == 0 {
				fmt.Fprintln(w, "No installed plugins declare model assets.")
				return nil
			}
			fmt.Fprintf(w, "%-14s %-18s %-8s %-11s %s\n", "PLUGIN", "MODEL", "VERSION", "STATUS", "SIZE")
			for _, pm := range models {
				status, size := modelStatus(pm.plugin, pm.asset)
				name := pm.asset.ID
				if pm.asset.Default {
					name += " (default)"
				}
				fmt.Fprintf(w, "%-14s %-18s %-8s %-11s %s\n", pm.plugin, name, pm.asset.Version, status, size)
			}
			return nil
		},
	}
}

func (a *App) newModelsPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <plugin>[/<model>]",
		Short: "Download and cache a plugin's model asset",
		Long: "Fetch and verify a model asset ahead of time so the first use of the plugin is\n" +
			"instant. With just <plugin>, pulls that plugin's default model.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plugin, modelID := splitModelRef(args[0])
			asset, ok := a.findModel(plugin, modelID)
			if !ok {
				return fmt.Errorf("no such model %q for plugin %q (see `kapi models list`)", args[0], plugin)
			}
			dir, err := EnsureModel(cmd.Context(), asset, ModelEnsureOptions{
				Plugin: plugin,
				Logf:   func(f string, a ...any) { fmt.Fprintf(cmd.ErrOrStderr(), f+"\n", a...) },
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s/%s ready at %s\n", plugin, asset.ID, dir)
			return nil
		},
	}
}

func (a *App) newModelsPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune <plugin>[/<model>]",
		Short: "Remove a cached model asset from disk",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plugin, modelID := splitModelRef(args[0])
			asset, ok := a.findModel(plugin, modelID)
			if !ok {
				return fmt.Errorf("no such model %q for plugin %q (see `kapi models list`)", args[0], plugin)
			}
			dir, err := ModelDir(plugin, asset.ID, asset.Version)
			if err != nil {
				return err
			}
			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s/%s is not cached.\n", plugin, asset.ID)
				return nil
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("prune %s/%s: %w", plugin, asset.ID, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ removed %s/%s (%s)\n", plugin, asset.ID, dir)
			return nil
		},
	}
}
