package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pluginreg "github.com/neokapi/neokapi/host/pluginhost/registry"

	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/spf13/cobra"
)

// NewPluginCmd creates the manifest-driven plugin command tree
// (singular `plugin`). This is the only plugin command tree — the
// legacy `plugins` (plural) command tree was removed in #438 phase 9
// when the v1 gRPC plugin runtime was deleted.
func NewPluginCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use: "plugin",
		// Accept "plugins" too — the plural reads naturally ("kapi plugins
		// install") and matches how the command is referenced across the docs.
		Aliases: []string{"plugins"},
		Short:   "Install and manage manifest-driven plugins",
		GroupID: "advanced",
	}

	cmd.AddCommand(newPluginListCmd(a))
	cmd.AddCommand(newPluginInfoCmd(a))
	cmd.AddCommand(newPluginInstallCmd(a))
	cmd.AddCommand(newPluginUpdateCmd(a))
	cmd.AddCommand(newPluginRemoveCmd(a))
	cmd.AddCommand(newPluginPruneCmd(a))
	cmd.AddCommand(newPluginSearchCmd(a))
	cmd.AddCommand(newPluginUpdateIndexCmd(a))
	cmd.AddCommand(newPluginRebuildCacheCmd(a))
	cmd.AddCommand(newPluginVerifyCmd(a))
	cmd.AddCommand(newPluginDoctorCmd(a))
	// Plugin registries are plugin configuration: `kapi plugin registry
	// list/add/remove` is the canonical (and only) home.
	registry := NewRegistryCmd(a)
	registry.GroupID = ""
	cmd.AddCommand(registry)
	return cmd
}

func newPluginListCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.PluginHost == nil {
				return errors.New("plugin host is not initialized")
			}
			plugins := a.PluginHost.Plugins()
			rows := make([]output.PluginListRow, 0, len(plugins))
			for _, p := range plugins {
				row := output.PluginListRow{
					Name:    p.Name(),
					Version: p.Version(),
					License: p.Manifest.License,
					Source:  p.Source.Label,
					Status:  "active",
				}
				if p.Retired != nil {
					row.Status = "retired"
					row.Retirement = p.Retired.Notice()
				}
				rows = append(rows, row)
			}
			return output.Print(cmd, output.PluginListOutput{Plugins: rows, Total: len(rows)})
		},
	}
}

func newPluginInfoCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show details for an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.PluginHost == nil {
				return errors.New("plugin host is not initialized")
			}
			p := a.PluginHost.Plugin(args[0])
			if p == nil {
				return fmt.Errorf("plugin %q is not installed", args[0])
			}
			c := p.Manifest.Capabilities
			return output.Print(cmd, output.PluginInfoOutput{
				Plugin:           p.Name(),
				Version:          p.Version(),
				License:          p.Manifest.License,
				Author:           p.Manifest.Author,
				Homepage:         p.Manifest.Homepage,
				InstallDir:       p.Dir,
				Source:           p.Source.Label,
				Binary:           p.BinaryPath,
				Commands:         len(c.Commands),
				MCPTools:         len(c.MCPTools),
				Formats:          len(c.Formats),
				SchemaExtensions: len(c.SchemaExtensions),
				Models:           len(p.Manifest.Models),
			})
		},
	}
}

func newPluginInstallCmd(a *App) *cobra.Command {
	var channel string
	var unsafe bool
	var indexURL string
	cmd := &cobra.Command{
		Use:   "install <name[@version]>",
		Short: "Install a plugin from the registry",
		Long: `Install a plugin from a registry. The plugin is downloaded,
verified (SHA-256 + cosign signature), and unpacked into
$XDG_DATA_HOME/kapi/plugins/<name>/.

Examples:
  kapi plugin install bowrain
  kapi plugin install bowrain@^1.0
  kapi plugin install bowrain --channel beta`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, constraint := ParsePluginRef(args[0])
			opts := pluginhost.InstallOptions{
				IndexURL:    indexURL,
				PluginName:  name,
				Constraint:  constraint,
				Channel:     channel,
				KapiVersion: KapiVersion(),
				Unsafe:      unsafe,
				LogF: func(msg string) {
					fmt.Fprintln(cmd.ErrOrStderr(), msg)
				},
			}
			// InstallPluginFromRegistry refuses a retired plugin (offline
			// tombstone), pointing at the replacement, before installing.
			result, err := InstallPluginFromRegistry(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s %s to %s\n", result.PluginName, result.Version, result.InstallDir)
			fmt.Fprintln(cmd.OutOrStdout(), "Run 'kapi plugin list' to verify, or 'kapi --help' to see new commands.")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "stable", "registry channel (e.g. stable, beta)")
	cmd.Flags().BoolVar(&unsafe, "unsafe", false, "skip SHA-256 and signature verification (install an unsigned/unverified plugin)")
	cmd.Flags().StringVar(&indexURL, "index", "", "registry index URL (default: $KAPI_REGISTRY_URL or builtin)")
	return cmd
}

// newPluginUpdateCmd implements `kapi plugin update <name>`.
//
// Reads <pluginDir>/installed.json to recover the channel, constraint,
// and index URL the plugin was originally installed from. Then runs
// the registry resolver against those same options. If the resolved
// version equals the installed version the command reports
// "already up to date"; otherwise it re-installs (which atomically
// replaces the on-disk plugin dir) and prints before/after versions.
func newPluginUpdateCmd(a *App) *cobra.Command {
	var channelOverride string
	var constraintOverride string
	var indexOverride string
	var unsafe bool
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an installed plugin to the latest matching version",
		Long: `Update an installed plugin in place using the channel and constraint
recorded at install time. Pass --channel or --constraint to switch
tracks during update; --index points the update at a different
registry index URL.

Examples:
  kapi plugin update bowrain
  kapi plugin update bowrain --channel beta
  kapi plugin update bowrain --constraint ^2.0`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			pluginDir := filepath.Join(pluginhost.InstallTarget(), name)
			if _, err := os.Stat(pluginDir); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("plugin %q is not installed under %s. Install it with `kapi plugin install %s`", name, pluginhost.InstallTarget(), name)
				}
				return err
			}

			result, currentVersion, err := UpdatePlugin(cmd.Context(), name, PluginUpdateOverrides{
				Channel:    channelOverride,
				Constraint: constraintOverride,
				IndexURL:   indexOverride,
				Unsafe:     unsafe,
				LogF: func(msg string) {
					fmt.Fprintln(cmd.ErrOrStderr(), msg)
				},
			})
			if err != nil {
				return err
			}
			if currentVersion == "" && a.PluginHost != nil {
				if p := a.PluginHost.Plugin(name); p != nil {
					currentVersion = p.Version()
				}
			}

			if currentVersion != "" && currentVersion == result.Version {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is already up to date (%s)\n", name, currentVersion)
				return nil
			}
			if currentVersion != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Updated %s %s → %s\n", name, currentVersion, result.Version)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Installed %s %s\n", name, result.Version)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&channelOverride, "channel", "", "registry channel (default: channel from installed.json)")
	cmd.Flags().StringVar(&constraintOverride, "constraint", "", "version constraint (default: constraint from installed.json)")
	cmd.Flags().StringVar(&indexOverride, "index", "", "registry index URL (default: index_url from installed.json)")
	cmd.Flags().BoolVar(&unsafe, "unsafe", false, "skip SHA-256 and signature verification (install an unsigned/unverified plugin)")
	return cmd
}

func newPluginRemoveCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.InitPluginHost()
			if a.PluginHost == nil {
				return fmt.Errorf("plugin %q is not installed", args[0])
			}
			if err := a.PluginHost.Remove(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", args[0])
			return nil
		},
	}
}

// newPluginPruneCmd implements `kapi plugins prune` — clean up plugins kapi has
// retired but that are still installed from a previous version. Retired plugins
// are already inert (never loaded); this removes them from disk. It never
// touches system (Homebrew) installs — it prints the OS command instead — and
// never removes a plugin's downloaded model cache or config.
func newPluginPruneCmd(a *App) *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove retired plugins that are still installed",
		Long: "Find plugins kapi has retired but that remain installed from an earlier version,\n" +
			"and remove them. Retired plugins are already inert (kapi never loads them); this\n" +
			"cleans them off disk after confirmation. System (Homebrew) installs are reported\n" +
			"with the command to remove them, never deleted by kapi. Downloaded model caches\n" +
			"and configuration are left untouched.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a.InitPluginHost()
			out := cmd.OutOrStdout()
			if a.PluginHost == nil {
				fmt.Fprintln(out, "Nothing to prune.")
				return nil
			}
			var retired []*pluginhost.Plugin
			for _, p := range a.PluginHost.Plugins() {
				if p.Retired != nil {
					retired = append(retired, p)
				}
			}
			if len(retired) == 0 {
				fmt.Fprintln(out, "Nothing to prune: no retired plugins are installed.")
				return nil
			}

			fmt.Fprintln(out, "Retired plugins still installed:")
			for _, p := range retired {
				fmt.Fprintf(out, "  • %s %s: %s\n      %s\n", p.Name(), p.Version(), p.Dir, p.Retired.Because)
			}
			if dryRun {
				fmt.Fprintln(out, "\n(dry run: nothing removed)")
				return nil
			}
			if !yes {
				ok, err := Confirm(cmd.InOrStdin(), out, "\nRemove these retired plugins? [Y/n] ")
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(out, "Aborted.")
					return nil
				}
			}

			var removed, skipped int
			for _, p := range retired {
				if err := a.PluginHost.Remove(p.Name()); err != nil {
					// System installs are guarded by Remove; report the OS command.
					fmt.Fprintf(out, "  skipped %s: %v\n", p.Name(), err)
					fmt.Fprintf(out, "      try: brew uninstall kapi-%s\n", p.Name())
					skipped++
					continue
				}
				fmt.Fprintf(out, "  removed %s\n", p.Name())
				removed++
			}
			fmt.Fprintf(out, "\nPruned %d, skipped %d.\n", removed, skipped)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "remove without confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list what would be removed, without removing")
	return cmd
}

func newPluginSearchCmd(a *App) *cobra.Command {
	var indexURL string
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search the registry for plugins",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			entries, err := SearchRegistry(cmd.Context(), indexURL, query, nil, true)
			if err != nil {
				return err
			}
			var platform string
			results := make([]output.PluginSearchEntry, 0, len(entries))
			for _, e := range entries {
				platform = e.Platform
				results = append(results, output.PluginSearchEntry{
					Name:        e.Name,
					Version:     e.Version,
					Description: e.Description,
					Installable: e.Installable,
				})
			}

			// --json gets the structured rows in full; the text form is a table
			// whose description column the renderer fits to the terminal.
			if output.ResolveFormat(cmd) == output.FormatJSON {
				return output.Print(cmd, output.PluginSearchOutput{Plugins: results, Total: len(results)})
			}
			t := output.NewTable(cmd.OutOrStdout()).Accent(0).
				Headers("PLUGIN", "VERSION", "DESCRIPTION")
			s := t.Styles()
			for _, r := range results {
				desc := r.Description
				if !r.Installable {
					desc += s.Warn.Render(fmt.Sprintf(" (no build for %s)", platform))
				}
				t.Row(r.Name, r.Version, desc)
			}
			t.Render()
			return nil
		},
	}
	cmd.Flags().StringVar(&indexURL, "index", "", "registry index URL")
	return cmd
}

func newPluginUpdateIndexCmd(a *App) *cobra.Command {
	var indexURL string
	cmd := &cobra.Command{
		Use:   "update-index",
		Short: "Refresh the cached registry index",
		RunE: func(cmd *cobra.Command, args []string) error {
			url := indexURL
			if url == "" {
				url = pluginhost.DefaultIndexURL()
			}
			_, err := pluginreg.FetchOrCached(cmd.Context(), url, true)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Refreshed registry index from", url)
			return nil
		},
	}
	cmd.Flags().StringVar(&indexURL, "index", "", "registry index URL")
	return cmd
}

func newPluginRebuildCacheCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "rebuild-cache",
		Short: "Force a rebuild of the plugin dispatch cache",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := pluginhost.DiscoverOptions{
				EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR"),
				OnWarn: func(s string) {
					fmt.Fprintln(cmd.ErrOrStderr(), "Warning:", s)
				},
			}
			plugins := pluginhost.Discover(opts)
			cache := pluginhost.BuildCache(opts, plugins)
			if err := pluginhost.SaveCache(pluginhost.CacheLocation(), cache); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rebuilt cache: %d plugin(s) → %s\n", len(plugins), pluginhost.CacheLocation())
			return nil
		},
	}
}

func newPluginVerifyCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "verify <name>",
		Short: "Re-verify an installed plugin's manifest and binary",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.PluginHost == nil {
				return errors.New("plugin host not initialized")
			}
			p := a.PluginHost.Plugin(args[0])
			if p == nil {
				return fmt.Errorf("plugin %q is not installed", args[0])
			}
			out, err := RunVersionProbe(cmd.Context(), p.BinaryPath)
			if err != nil {
				return fmt.Errorf("plugin %q: version probe failed: %w", p.Name(), err)
			}
			declared := p.Manifest.Version
			actual := strings.TrimSpace(string(out))
			if actual != declared {
				return fmt.Errorf("plugin %q: manifest version %q ≠ binary version %q", p.Name(), declared, actual)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s %s OK\n", p.Name(), declared)
			return nil
		},
	}
}

// newPluginDoctorCmd implements `kapi plugins doctor [name]`.
//
// Doctor is the single, consistent health surface for installed plugins. It
// replaces the per-plugin self-check verbs (the old `kapi av`, `kapi asr`,
// `kapi vision`, …) that each plugin used to mint as a top-level command. For
// every plugin it confirms the binary is present and its reported version
// matches the manifest, then — for plugins that declare a self-check
// (capabilities.selfcheck) — runs the plugin's own `<binary> doctor`
// diagnostics, which Confirm bundled binaries, models, or engines resolve at
// runtime.
//
// With no argument it checks every installed plugin and prints a one-line
// status each. With a name it prints a detailed report including the plugin's
// full self-check output. Exits non-zero if any checked plugin is unhealthy.
func newPluginDoctorCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor [name]",
		Short: "Run health/self-checks on installed plugins",
		Long: `Check the health of installed plugins.

For each plugin, doctor verifies the binary is present and its reported
version matches the manifest, then, for plugins that provide a
self-check, runs the plugin's own diagnostics (e.g. confirming bundled
binaries, models, or engines resolve at runtime).

With no argument, doctor checks every installed plugin and prints a
one-line status each. Pass a plugin name for a detailed report including
the plugin's full self-check output. Exits non-zero if any checked
plugin is unhealthy.

Examples:
  kapi plugins doctor
  kapi plugins doctor av`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.InitPluginHost()
			if a.PluginHost == nil {
				return errors.New("plugin host is not initialized")
			}

			var targets []*pluginhost.Plugin
			if len(args) == 1 {
				p := a.PluginHost.Plugin(args[0])
				if p == nil {
					return fmt.Errorf("plugin %q is not installed", args[0])
				}
				targets = []*pluginhost.Plugin{p}
			} else {
				targets = a.PluginHost.Plugins()
			}

			if len(targets) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No plugins installed.")
				return nil
			}

			verbose := len(args) == 1
			unhealthy := 0
			t := output.NewTable(cmd.OutOrStdout()).Accent(0).
				Headers("PLUGIN", "VERSION", "HEALTH")
			s := t.Styles()
			for _, p := range targets {
				res := DiagnosePlugin(cmd.Context(), p)
				if !res.Healthy {
					unhealthy++
				}
				if verbose {
					WriteDoctorReport(cmd.OutOrStdout(), p, res)
					continue
				}
				health := s.Success.Render("✓ " + res.Summary)
				if !res.Healthy {
					health = s.Error.Render("✗ " + res.Summary)
				}
				t.Row(p.Name(), p.Version(), health)
			}
			t.Render()

			if unhealthy > 0 {
				return WithExitCode(1, fmt.Errorf("%d of %d plugin(s) unhealthy", unhealthy, len(targets)))
			}
			return nil
		},
	}
}
