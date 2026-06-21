package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/muesli/reflow/wordwrap"
	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/cli/pluginhost"
	pluginreg "github.com/neokapi/neokapi/cli/pluginhost/registry"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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
	cmd.AddCommand(a.newPluginDoctorCmd())
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
			rows := make([]output.PluginListRow, 0, len(plugins))
			for _, p := range plugins {
				rows = append(rows, output.PluginListRow{
					Name:    p.Name(),
					Version: p.Version(),
					License: p.Manifest.License,
					Source:  p.Source.Label,
				})
			}
			return output.Print(cmd, output.PluginListOutput{Plugins: rows, Total: len(rows)})
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

			// Sort by name for stable, scannable output (the index is a map).
			names := make([]string, 0, len(idx.Plugins))
			for name := range idx.Plugins {
				names = append(names, name)
			}
			sort.Strings(names)

			results := make([]output.PluginSearchEntry, 0, len(names))
			for _, name := range names {
				entry := idx.Plugins[name]
				if query != "" && !strings.Contains(strings.ToLower(name), query) && !strings.Contains(strings.ToLower(entry.Description), query) {
					continue
				}
				latest := ""
				for v := range entry.Versions {
					if latest == "" || pluginreg.CompareSemver(v, latest) > 0 {
						latest = v
					}
				}
				// Flag plugins with no installable build for this OS/arch, mirroring
				// the install path's resolution — so `install` won't fail with a raw
				// "no version ... for platform" error after a misleading listing.
				_, _, rerr := idx.Resolve(name, "", "stable", version.Version)
				results = append(results, output.PluginSearchEntry{
					Name:        name,
					Version:     latest,
					Description: entry.Description,
					Installable: rerr == nil,
				})
			}

			// --json gets the structured rows; the text form keeps the word-wrapped
			// description layout (a flat table would mangle long descriptions).
			if output.ResolveFormat(cmd) == output.FormatJSON {
				return output.Print(cmd, output.PluginSearchOutput{Plugins: results, Total: len(results)})
			}
			const prefixWidth = 32 // "%-20s %-10s " → 20 + 1 + 10 + 1
			descWidth := descriptionWidth(cmd.OutOrStdout(), prefixWidth)
			indent := strings.Repeat(" ", prefixWidth)
			for _, r := range results {
				desc := r.Description
				if !r.Installable {
					desc += fmt.Sprintf(" (no build for %s)", platform)
				}
				wrapped := wrapText(desc, descWidth)
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %s\n", r.Name, r.Version, wrapped[0])
				for _, cont := range wrapped[1:] {
					fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", indent, cont)
				}
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

// newPluginDoctorCmd implements `kapi plugins doctor [name]`.
//
// Doctor is the single, consistent health surface for installed plugins. It
// replaces the per-plugin self-check verbs (the old `kapi av`, `kapi asr`,
// `kapi vision`, …) that each plugin used to mint as a top-level command. For
// every plugin it confirms the binary is present and its reported version
// matches the manifest, then — for plugins that declare a self-check
// (capabilities.selfcheck) — runs the plugin's own `<binary> doctor`
// diagnostics, which confirm bundled binaries, models, or engines resolve at
// runtime.
//
// With no argument it checks every installed plugin and prints a one-line
// status each. With a name it prints a detailed report including the plugin's
// full self-check output. Exits non-zero if any checked plugin is unhealthy.
func (a *App) newPluginDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor [name]",
		Short: "Run health/self-checks on installed plugins",
		Long: `Check the health of installed plugins.

For each plugin, doctor verifies the binary is present and its reported
version matches the manifest, then — for plugins that provide a
self-check — runs the plugin's own diagnostics (e.g. confirming bundled
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
			for _, p := range targets {
				res := diagnosePlugin(cmd.Context(), p)
				if !res.healthy {
					unhealthy++
				}
				if verbose {
					writeDoctorReport(cmd.OutOrStdout(), p, res)
				} else {
					mark := "✓"
					if !res.healthy {
						mark = "✗"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s %-20s %-10s %s\n", mark, p.Name(), p.Version(), res.summary)
				}
			}

			if unhealthy > 0 {
				return WithExitCode(1, fmt.Errorf("%d of %d plugin(s) unhealthy", unhealthy, len(targets)))
			}
			return nil
		},
	}
}

// doctorResult is the outcome of diagnosing a single plugin.
type doctorResult struct {
	healthy bool
	summary string   // one-line status detail
	checks  []string // per-check lines for the verbose report
	output  string   // captured self-check stdout/stderr (verbose only)
}

// diagnosePlugin runs the baseline + self-check diagnostics for one plugin:
//  1. the binary file exists and is a regular file,
//  2. `<binary> version` reports a version matching the manifest,
//  3. if the manifest declares a self-check, `<binary> doctor` exits 0.
func diagnosePlugin(ctx context.Context, p *pluginhost.Plugin) doctorResult {
	res := doctorResult{healthy: true}

	if st, err := os.Stat(p.BinaryPath); err != nil || st.IsDir() {
		res.healthy = false
		res.summary = "binary missing: " + p.BinaryPath
		res.checks = append(res.checks, "✗ binary present ("+p.BinaryPath+")")
		return res
	}
	res.checks = append(res.checks, "✓ binary present")

	// The version probe is an integrity hint, not a health gate: it runs across
	// every installed plugin — including third-party ones whose `version`
	// subcommand may print extra text or be absent — so a mismatch is surfaced
	// as a warning, never as "unhealthy". Unhealthy is reserved for a missing
	// binary or a failing declared self-check.
	out, err := runVersionProbe(ctx, p.BinaryPath)
	actual := strings.TrimSpace(string(out))
	switch {
	case err != nil:
		res.checks = append(res.checks, fmt.Sprintf("⚠ version probe failed: %v", err))
	case actual != p.Manifest.Version:
		res.checks = append(res.checks, fmt.Sprintf("⚠ version probe %q ≠ manifest %q", actual, p.Manifest.Version))
	default:
		res.checks = append(res.checks, "✓ version matches manifest ("+actual+")")
	}

	if !p.Manifest.Capabilities.SelfCheck {
		res.checks = append(res.checks, "– no self-check")
		res.summary = "ok (no self-check)"
		return res
	}

	scOut, scErr := runSelfCheckProbe(ctx, p.BinaryPath)
	res.output = strings.TrimRight(string(scOut), "\n")
	if scErr != nil {
		res.healthy = false
		res.checks = append(res.checks, fmt.Sprintf("✗ self-check failed: %v", scErr))
		res.summary = "self-check failed"
		return res
	}
	res.checks = append(res.checks, "✓ self-check passed")
	res.summary = "healthy"
	return res
}

// writeDoctorReport prints the detailed per-plugin diagnostic report.
func writeDoctorReport(w io.Writer, p *pluginhost.Plugin, res doctorResult) {
	status := "healthy"
	if !res.healthy {
		status = "UNHEALTHY"
	}
	fmt.Fprintf(w, "%s %s — %s\n", p.Name(), p.Version(), status)
	fmt.Fprintf(w, "  binary: %s\n", p.BinaryPath)
	for _, c := range res.checks {
		fmt.Fprintf(w, "  %s\n", c)
	}
	if res.output != "" {
		fmt.Fprintln(w, "  self-check output:")
		for line := range strings.SplitSeq(res.output, "\n") {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}
}

// runSelfCheckProbe runs the plugin's standard `<binary> doctor` self-check,
// capturing stdout+stderr. A non-nil error means the self-check exited non-zero
// (or the binary could not be run).
func runSelfCheckProbe(ctx context.Context, binPath string) ([]byte, error) {
	return exec.CommandContext(ctx, binPath, "doctor").CombinedOutput()
}

// descriptionWidth returns the width available for the wrapped description
// column: the terminal width minus prefixWidth. It returns 0 when w is not a
// terminal (piped/redirected output) or the width can't be determined, or when
// the remaining column would be too narrow to wrap usefully — callers treat 0 as
// "don't wrap, print the description in full on one line".
func descriptionWidth(w io.Writer, prefixWidth int) int {
	f, ok := w.(interface{ Fd() uintptr })
	if !ok || !isatty.IsTerminal(f.Fd()) {
		return 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	avail := cols - prefixWidth
	if avail < 24 { // too narrow to wrap into; print full lines instead
		return 0
	}
	return avail
}

// wrapText word-wraps s into lines no wider than width cells, breaking only at
// spaces (words longer than width are kept whole). A width of 0 yields the text
// unbroken. It always returns at least one line. Wrapping is delegated to
// muesli/reflow, which is wide-rune- and ANSI-aware.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	return strings.Split(wordwrap.String(s, width), "\n")
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
