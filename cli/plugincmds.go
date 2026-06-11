package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/cli/pluginhost"
	pluginreg "github.com/neokapi/neokapi/cli/pluginhost/registry"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

// NewPluginCmd creates the manifest-driven plugin command tree
// (singular `plugin`). This is the only plugin command tree — the
// legacy `plugins` (plural) command tree was removed in #438 phase 9
// when the v1 gRPC plugin runtime was deleted.
func (a *App) NewPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "plugin",
		// Accept "plugins" too — the plural reads naturally ("kapi plugins
		// install") and matches how the command is referenced across the docs.
		Aliases: []string{"plugins"},
		Short:   "Install and manage manifest-driven plugins (#438)",
		GroupID: "management",
	}

	cmd.AddCommand(a.newPluginListCmd())
	cmd.AddCommand(a.newPluginInfoCmd())
	cmd.AddCommand(a.newPluginInstallCmd())
	cmd.AddCommand(a.newPluginUpdateCmd())
	cmd.AddCommand(a.newPluginRemoveCmd())
	cmd.AddCommand(a.newPluginSearchCmd())
	cmd.AddCommand(a.newPluginUpdateIndexCmd())
	cmd.AddCommand(a.newPluginRebuildCacheCmd())
	cmd.AddCommand(a.newPluginVerifyCmd())
	return cmd
}

func (a *App) newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.PluginHost == nil {
				return errors.New("plugin host is not initialized")
			}
			plugins := a.PluginHost.Plugins()
			if len(plugins) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No plugins installed.")
				fmt.Fprintln(cmd.OutOrStdout(), "Search the registry: kapi plugin search")
				return nil
			}
			for _, p := range plugins {
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-12s %s\n",
					p.Name(), p.Version(), p.Manifest.License, p.Source.Label)
			}
			return nil
		},
	}
}

func (a *App) newPluginInfoCmd() *cobra.Command {
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
			fmt.Fprintf(cmd.OutOrStdout(), "Plugin:        %s\n", p.Name())
			fmt.Fprintf(cmd.OutOrStdout(), "Version:       %s\n", p.Version())
			fmt.Fprintf(cmd.OutOrStdout(), "License:       %s\n", p.Manifest.License)
			if p.Manifest.Author != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Author:        %s\n", p.Manifest.Author)
			}
			if p.Manifest.Homepage != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Homepage:      %s\n", p.Manifest.Homepage)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Install dir:   %s\n", p.Dir)
			fmt.Fprintf(cmd.OutOrStdout(), "Source:        %s\n", p.Source.Label)
			fmt.Fprintf(cmd.OutOrStdout(), "Binary:        %s\n", p.BinaryPath)
			c := p.Manifest.Capabilities
			if len(c.Commands) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Commands:      %d\n", len(c.Commands))
			}
			if len(c.MCPTools) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "MCP tools:     %d\n", len(c.MCPTools))
			}
			if len(c.Formats) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Formats:       %d\n", len(c.Formats))
			}
			if len(c.SchemaExtensions) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Schema exts:   %d\n", len(c.SchemaExtensions))
			}
			return nil
		},
	}
}

func (a *App) newPluginInstallCmd() *cobra.Command {
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
			name, constraint := parsePluginRef(args[0])
			opts := pluginhost.InstallOptions{
				IndexURL:    indexURL,
				PluginName:  name,
				Constraint:  constraint,
				Channel:     channel,
				KapiVersion: kapiVersion(),
				Unsafe:      unsafe,
				LogF: func(msg string) {
					fmt.Fprintln(cmd.ErrOrStderr(), msg)
				},
			}
			result, err := pluginhost.InstallFromRegistry(cmd.Context(), opts)
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
func (a *App) newPluginUpdateCmd() *cobra.Command {
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
					return fmt.Errorf("plugin %q is not installed under %s — install with `kapi plugin install %s`", name, pluginhost.InstallTarget(), name)
				}
				return err
			}

			meta, err := pluginhost.ReadInstalledMetadata(pluginDir)
			switch {
			case err != nil && os.IsNotExist(err):
				// No bookkeeping file (legacy install or dev plugin).
				// Fall back to channel/constraint/index from flags or
				// defaults.
				meta = &pluginhost.InstalledMetadata{}
			case err != nil:
				return err
			}

			channel := channelOverride
			if channel == "" {
				channel = meta.Channel
			}
			constraint := constraintOverride
			if constraint == "" {
				constraint = meta.Constraint
			}
			indexURL := indexOverride
			if indexURL == "" {
				indexURL = meta.IndexURL
			}

			currentVersion := meta.Version
			if currentVersion == "" && a.PluginHost != nil {
				if p := a.PluginHost.Plugin(name); p != nil {
					currentVersion = p.Version()
				}
			}

			result, err := pluginhost.InstallFromRegistry(cmd.Context(), pluginhost.InstallOptions{
				IndexURL:    indexURL,
				PluginName:  name,
				Constraint:  constraint,
				Channel:     channel,
				KapiVersion: kapiVersion(),
				Unsafe:      unsafe,
				LogF: func(msg string) {
					fmt.Fprintln(cmd.ErrOrStderr(), msg)
				},
			})
			if err != nil {
				return err
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

func (a *App) newPluginRemoveCmd() *cobra.Command {
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

func (a *App) newPluginSearchCmd() *cobra.Command {
	var indexURL string
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search the registry for plugins",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = strings.ToLower(args[0])
			}
			url := indexURL
			if url == "" {
				url = pluginhost.DefaultIndexURL()
			}
			idx, err := pluginreg.FetchOrCached(cmd.Context(), url, true)
			if err != nil {
				return err
			}
			platform := pluginreg.PlatformKey()
			for name, entry := range idx.Plugins {
				if query != "" && !strings.Contains(strings.ToLower(name), query) && !strings.Contains(strings.ToLower(entry.Description), query) {
					continue
				}
				latest := ""
				for v := range entry.Versions {
					if latest == "" || pluginreg.CompareSemver(v, latest) > 0 {
						latest = v
					}
				}
				line := fmt.Sprintf("%-20s %-10s %s", name, latest, entry.Description)
				// Flag plugins with no installable build for this OS/arch, mirroring
				// the install path's resolution — so `install` won't fail with a raw
				// "no version ... for platform" error after a misleading listing.
				if _, _, err := idx.Resolve(name, "", "stable", version.Version); err != nil {
					line += fmt.Sprintf("  (no build for %s)", platform)
				}
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&indexURL, "index", "", "registry index URL")
	return cmd
}

func (a *App) newPluginUpdateIndexCmd() *cobra.Command {
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

func (a *App) newPluginRebuildCacheCmd() *cobra.Command {
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

func (a *App) newPluginVerifyCmd() *cobra.Command {
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
			out, err := runVersionProbe(cmd.Context(), p.BinaryPath)
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

// parsePluginRef splits "name@^1.0" → ("name", "^1.0").
func parsePluginRef(ref string) (name, constraint string) {
	if before, after, ok := strings.Cut(ref, "@"); ok {
		return before, after
	}
	return ref, ""
}

func kapiVersion() string {
	return version.Version
}

func runVersionProbe(ctx context.Context, binPath string) ([]byte, error) {
	return exec.CommandContext(ctx, binPath, "version").CombinedOutput()
}
