package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/cli/pluginhost"
	pluginreg "github.com/neokapi/neokapi/cli/pluginhost/registry"
	"github.com/neokapi/neokapi/core/project"
)

// LoadProjectInteractive loads a .kapi recipe and, when the host binary
// is missing one or more declared `requires:` extensions, offers an
// interactive auto-install prompt before failing.
//
// Resolution flow:
//
//  1. project.LoadWithOptions(SkipRequiresCheck: true) — parse + run
//     full schema validation but skip the "extension group registered"
//     check.
//  2. For every entry in proj.MissingRequires(): if --yes is set or
//     stdin is a TTY, prompt to install. On confirm, call
//     pluginhost.InstallFromRegistry, then re-discover plugins on the
//     App's PluginHost and register their schema_extensions so
//     project.HasExtensionGroup reports them present.
//  3. proj.ValidateRequires() — fails with the original actionable
//     error string when a missing extension wasn't installed.
//
// Non-TTY callers without --yes get the same actionable error
// kapi has always produced.
type LoadProjectInteractiveOptions struct {
	// AssumeYes short-circuits the interactive confirm. Wired to the
	// global --yes / -y flag.
	AssumeYes bool

	// In is the source of confirmation answers. Defaults to os.Stdin.
	In io.Reader

	// Out is where the prompt is written. Defaults to os.Stderr (so
	// command output piped to a file is uncluttered).
	Out io.Writer

	// IsTTYFn lets tests override TTY detection. Defaults to checking
	// os.Stdin via mattn/go-isatty.
	IsTTYFn func() bool

	// InstallFn lets tests inject a fake installer. When nil,
	// pluginhost.InstallFromRegistry is used.
	InstallFn func(ctx context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error)

	// IndexURL overrides the registry URL used to fetch plugin
	// metadata for the prompt. Empty falls back to
	// pluginhost.DefaultIndexURL().
	IndexURL string

	// Channel pins the registry channel used to look up the plugin
	// version satisfying the recipe constraint. Empty defaults to
	// "stable".
	Channel string
}

// LoadProjectInteractive is the wrapper every project-aware command
// uses to load a recipe. It honors auto-install UX, falls back to the
// existing actionable error on non-TTY, and re-validates after install.
func (a *App) LoadProjectInteractive(ctx context.Context, recipePath string, opts LoadProjectInteractiveOptions) (*project.KapiProject, error) {
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, err
	}

	missing := proj.MissingRequires()
	if len(missing) == 0 {
		return proj, nil
	}

	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}
	isTTY := opts.IsTTYFn
	if isTTY == nil {
		isTTY = defaultIsStdinTTY
	}
	installFn := opts.InstallFn
	if installFn == nil {
		installFn = pluginhost.InstallFromRegistry
	}

	indexURL := opts.IndexURL
	if indexURL == "" {
		indexURL = pluginhost.DefaultIndexURL()
	}
	channel := opts.Channel
	if channel == "" {
		channel = "stable"
	}

	for _, req := range missing {
		// In non-interactive mode without --yes, surface the original
		// error. Mirrors the message produced by Validate so existing
		// tooling and docs continue to point users to the same fix.
		if !opts.AssumeYes && !isTTY() {
			return nil, fmt.Errorf("recipe requires plugin %q (%s) but no matching extension is registered (install with `kapi plugin install %s`)",
				req.Plugin, req.Constraint, req.Plugin)
		}

		// Show metadata pulled from the registry index, then prompt.
		info := lookupPluginInfo(ctx, indexURL, req.Plugin, req.Constraint, channel, kapiVersion())
		printPluginPromptHeader(out, req, info)

		if !opts.AssumeYes {
			ok, err := confirm(in, out, fmt.Sprintf("Install %s now? [Y/n] ", req.Plugin))
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("recipe requires plugin %q (%s) but no matching extension is registered (install with `kapi plugin install %s`)",
					req.Plugin, req.Constraint, req.Plugin)
			}
		}

		installOpts := pluginhost.InstallOptions{
			IndexURL:    indexURL,
			PluginName:  req.Plugin,
			Constraint:  req.Constraint,
			Channel:     channel,
			KapiVersion: kapiVersion(),
			LogF: func(msg string) {
				fmt.Fprintln(out, msg)
			},
		}
		result, err := installFn(ctx, installOpts)
		if err != nil {
			return nil, fmt.Errorf("auto-install %s: %w", req.Plugin, err)
		}
		fmt.Fprintf(out, "Installed %s %s to %s\n", result.PluginName, result.Version, result.InstallDir)

		// Re-discover plugins on the host so subsequent dispatch (and
		// schema validation) sees the freshly-installed plugin.
		a.refreshPluginHost()
	}

	// Re-validate the requires block now that we've (hopefully)
	// registered every missing extension. Surfaces the original
	// actionable error if anything is still missing — e.g. the user
	// declined a follow-up prompt or the install resolved to a plugin
	// whose manifest doesn't register the expected extension group.
	if err := proj.ValidateRequires(); err != nil {
		return nil, err
	}
	return proj, nil
}

// refreshPluginHost re-runs plugin discovery and schema-extension
// registration so the framework sees plugins installed mid-execution.
// Cache is bypassed here — the install path mutates plugin dirs that
// the cache may have stamped.
func (a *App) refreshPluginHost() {
	opts := pluginhost.DiscoverOptions{
		EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR"),
		OnWarn: func(s string) {
			if !a.Quiet {
				fmt.Fprintln(os.Stderr, "Warning: "+s)
			}
		},
	}
	plugins := pluginhost.Discover(opts)
	// Best-effort cache rewrite so the next process startup sees the
	// new plugin without rescanning.
	_ = pluginhost.SaveCache(pluginhost.CacheLocation(), pluginhost.BuildCache(opts, plugins))

	a.PluginHost = pluginhost.NewHost(plugins, func(s string) {
		if !a.Quiet {
			fmt.Fprintln(os.Stderr, "Warning: "+s)
		}
	})
	pluginhost.RegisterSchemaExtensions(a.PluginHost, func(s string) {
		if !a.Quiet {
			fmt.Fprintln(os.Stderr, "Warning: "+s)
		}
	})
}

// confirm reads a Y/N answer from r, accepting empty (default yes), Y,
// y, yes; rejects N, n, no.
func confirm(r io.Reader, w io.Writer, prompt string) (bool, error) {
	fmt.Fprint(w, prompt)
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	switch answer {
	case "", "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// pluginPromptInfo summarizes the metadata kapi shows before the
// auto-install prompt. Fields are best-effort: when the registry is
// unreachable we still prompt, just without the fancy header.
type pluginPromptInfo struct {
	Description string
	Author      string
	License     string
	Homepage    string
	Channel     string
	Version     string
}

func lookupPluginInfo(ctx context.Context, indexURL, name, constraint, channel, kapiVersion string) pluginPromptInfo {
	idx, err := pluginreg.FetchOrCached(ctx, indexURL, false)
	if err != nil {
		return pluginPromptInfo{}
	}
	entry, ok := idx.Plugins[name]
	if !ok {
		return pluginPromptInfo{}
	}
	out := pluginPromptInfo{
		Description: entry.Description,
		Author:      entry.Author,
		License:     entry.License,
		Homepage:    entry.Homepage,
	}
	// Best matching version for the prompt — same semantics as install.
	if v, _, err := idx.Resolve(name, constraint, channel, kapiVersion); err == nil {
		out.Version = v
		if ve, ok := entry.Versions[v]; ok && ve.Channel != "" {
			out.Channel = ve.Channel
		} else {
			out.Channel = channel
		}
	}
	return out
}

func printPluginPromptHeader(w io.Writer, req project.MissingRequirement, info pluginPromptInfo) {
	fmt.Fprintf(w, "\nRecipe requires plugin %q (%s).\n", req.Plugin, req.Constraint)
	if info.Version != "" {
		fmt.Fprintf(w, "  Version:   %s\n", info.Version)
	}
	if info.Description != "" {
		fmt.Fprintf(w, "  Summary:   %s\n", info.Description)
	}
	if info.Author != "" {
		fmt.Fprintf(w, "  Author:    %s\n", info.Author)
	}
	if info.License != "" {
		fmt.Fprintf(w, "  License:   %s\n", info.License)
	}
	if info.Homepage != "" {
		fmt.Fprintf(w, "  Homepage:  %s\n", info.Homepage)
	}
	if info.Channel != "" {
		fmt.Fprintf(w, "  Channel:   %s\n", info.Channel)
	}
}

// defaultIsStdinTTY is the default IsTTYFn — checks whether os.Stdin
// is attached to a terminal. Tests can swap this via
// LoadProjectInteractiveOptions.IsTTYFn.
func defaultIsStdinTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
}
