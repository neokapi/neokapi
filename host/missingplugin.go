package host

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/spf13/pflag"
)

// PluginHint is the little kapi knows, offline, about a plugin it does not
// have installed: the top-level recipe key the plugin owns, and the verbs it
// contributes to the command tree.
//
// Dispatch is never driven from here — that stays manifest-driven, read from
// the installed plugin. This table exists for the one moment when there is no
// manifest to read: the user typed a verb kapi does not have, and kapi has to
// decide whether that is a typo or a missing plugin. Without it, `kapi push`
// in a project whose recipe declares `bowrain:` fails with cobra's bare
// `unknown command "push"`, which names neither the cause nor the fix.
//
// The contents are pinned to the plugin's real manifest by a drift test in the
// plugin's own module (bowrain/plugin/cmd/kapi-bowrain), so adding or removing
// a bowrain verb fails the build until this table follows.
type PluginHint struct {
	// Plugin is the registry name, as `kapi plugin install <name>` takes it.
	Plugin string

	// RecipeKey is the top-level kapi.yaml key whose presence means this
	// project needs the plugin. It survives in KapiProject.Extras when the
	// plugin that owns the schema extension is absent.
	RecipeKey string

	// Verbs are the top-level commands the plugin contributes that do not
	// collide with a built-in. Group-scoped verbs (where a built-in keeps
	// the top-level spelling) and hidden plumbing are excluded: neither can
	// ever surface as an unknown command.
	Verbs []string

	// Formula is the Homebrew formula that installs the plugin, offered
	// alongside `kapi plugin install` for users who manage kapi with brew.
	Formula string
}

// pluginHints is the whole table. One entry today; the shape is per-plugin so
// a second manifest-driven plugin with its own recipe key needs no new code.
var pluginHints = []PluginHint{{
	Plugin:    "bowrain",
	RecipeKey: "bowrain",
	Verbs:     []string{"push", "pull", "diff", "auth", "stream", "workspace", "ui"},
	Formula:   "neokapi/tap/bowrain-cli",
}}

// PluginHintForVerb returns the hint for the plugin that provides verb, and
// whether one exists. Exported for the drift test that pins the table to the
// plugin manifest.
func PluginHintForVerb(verb string) (PluginHint, bool) {
	for _, h := range pluginHints {
		if slices.Contains(h.Verbs, verb) {
			return h, true
		}
	}
	return PluginHint{}, false
}

// PluginHintFor returns the hint for a plugin by name.
func PluginHintFor(plugin string) (PluginHint, bool) {
	for _, h := range pluginHints {
		if h.Plugin == plugin {
			return h, true
		}
	}
	return PluginHint{}, false
}

// MissingPluginOptions configures ResolveMissingPluginCommand. The zero value
// is what the CLI uses; tests override the seams.
type MissingPluginOptions struct {
	// In is the source of confirmation answers. Defaults to os.Stdin.
	In io.Reader

	// Out is where the offer and install progress are written. Defaults to
	// os.Stderr, so a command whose stdout is piped stays clean.
	Out io.Writer

	// IsTTYFn overrides TTY detection. Defaults to checking os.Stdin.
	IsTTYFn func() bool

	// InstallFn overrides the installer. Defaults to
	// pluginhost.InstallFromRegistry.
	InstallFn func(ctx context.Context, opts pluginhost.InstallOptions) (*pluginhost.InstallResult, error)

	// DispatchFn overrides what runs after a successful install. Defaults to
	// dispatching the verb through the freshly discovered plugin route.
	DispatchFn func(ctx context.Context, verb string, args []string) error

	// RecipeExtras overrides project discovery. When nil the recipe is
	// resolved from cmd the same way every project-aware command resolves it.
	RecipeExtras map[string]any

	// Argv is the process arguments after the binary name. Cobra rejects an
	// unknown verb inside Find(), which returns before execute() parses any
	// flags — so on this path App.AssumeYes and App.Quiet are still zero no
	// matter what the user wrote. When Argv is set, --yes and --quiet are
	// recovered from it so they mean what they say here too. Tests set the
	// App fields directly and leave this nil.
	Argv []string

	// IndexURL overrides the registry index URL.
	IndexURL string

	// Channel pins the registry channel. Empty defaults to "stable".
	Channel string
}

// ResolveMissingPluginCommand turns cobra's `unknown command "push"` into an
// actionable outcome when the verb belongs to a plugin this project needs but
// does not have installed.
//
// It reports handled=false — leaving the original error untouched — unless all
// of the following hold: the verb belongs to a known plugin, no installed
// plugin already routes it, and the project's recipe declares the key that
// plugin owns. A typo stays a typo, and `kapi push` outside a server-backed
// project keeps cobra's error and its "did you mean" suggestions.
//
// When it does engage:
//
//   - With --yes, or on an interactive terminal where the user accepts, the
//     plugin is installed through the same registry path as
//     `kapi plugin install`, plugins are re-discovered, and the original verb
//     runs — the user's command completes rather than merely being explained.
//   - Otherwise (CI, a pipe, --quiet, or a declined prompt) it returns an
//     error carrying both install instructions, exit code ExitUsage.
func (a *App) ResolveMissingPluginCommand(cmd Command, verb string, args []string, opts MissingPluginOptions) (bool, error) {
	hint, ok := PluginHintForVerb(verb)
	if !ok {
		return false, nil
	}
	// An installed plugin already routing this verb means the failure was
	// something else entirely; leave it alone.
	if a.PluginHost != nil && a.PluginHost.CommandRoute(verb) != nil {
		return false, nil
	}
	if !a.recipeDeclares(cmd, hint.RecipeKey, opts.RecipeExtras) {
		return false, nil
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

	assumeYes, quiet := a.AssumeYes, a.Quiet
	if opts.Argv != nil {
		assumeYes, quiet = recoverConfirmFlags(opts.Argv, assumeYes, quiet)
	}

	// --quiet means "no chatter"; a prompt is chatter. Treat it, and any
	// non-terminal stdin, as non-interactive.
	interactive := !quiet && isTTY()
	if !assumeYes && !interactive {
		return true, WithExitCode(ExitUsage, missingPluginError(hint, verb))
	}

	if !quiet {
		fmt.Fprint(out, missingPluginOffer(hint, verb))
	}
	if !assumeYes {
		okToInstall, err := Confirm(in, out, fmt.Sprintf("Install %s now? [Y/n] ", hint.Plugin))
		if err != nil {
			return true, err
		}
		if !okToInstall {
			return true, WithExitCode(ExitUsage, missingPluginError(hint, verb))
		}
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

	logF := func(msg string) { fmt.Fprintln(out, msg) }
	if quiet {
		logF = nil
	}
	if _, err := installFn(cmd.Context(), pluginhost.InstallOptions{
		IndexURL:    indexURL,
		PluginName:  hint.Plugin,
		Channel:     channel,
		KapiVersion: KapiVersion(),
		LogF:        logF,
	}); err != nil {
		return true, fmt.Errorf("install %s: %w", hint.Plugin, err)
	}
	if !quiet {
		fmt.Fprintf(out, "\nRunning `kapi %s`...\n\n", verb)
	}

	// Re-discover so the verb the user typed can finally run.
	a.refreshPluginHost()

	dispatch := opts.DispatchFn
	if dispatch == nil {
		dispatch = a.dispatchPluginVerb
	}
	return true, dispatch(cmd.Context(), verb, args)
}

// recoverConfirmFlags reads --yes/-y and --quiet/-q back out of argv.
//
// It exists because cobra never parsed them: an unknown verb is rejected in
// Find(), which returns before execute() reaches ParseFlags, so every global
// flag on this invocation is still at its default. A throwaway flag set with
// unknown flags whitelisted lets the verb's own flags (`kapi push --force`)
// pass by without derailing the scan, and it never touches the real command's
// flag state.
func recoverConfirmFlags(argv []string, assumeYes, quiet bool) (bool, bool) {
	fs := pflag.NewFlagSet("global", pflag.ContinueOnError)
	fs.ParseErrorsAllowlist.UnknownFlags = true
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	yes := fs.BoolP("yes", "y", assumeYes, "")
	q := fs.BoolP("quiet", "q", quiet, "")
	if err := fs.Parse(argv); err != nil {
		// Best effort: whatever was parsed before the error still counts.
		return *yes, *q
	}
	return *yes, *q
}

// recipeDeclares reports whether the project recipe in scope carries the
// given top-level key. A plugin-owned key survives in Extras precisely
// because the plugin that would decode it is not installed.
func (a *App) recipeDeclares(cmd Command, key string, override map[string]any) bool {
	if override != nil {
		_, ok := override[key]
		return ok
	}
	path, err := ResolveProjectPath(cmd)
	if err != nil || path == "" {
		return false
	}
	proj, err := project.LoadWithOptions(path, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return false
	}
	_, ok := proj.Extras[key]
	return ok
}

// dispatchPluginVerb runs verb through the freshly installed plugin, the same
// way pluginattach's synthesized cobra command would have: raw args through to
// the plugin, over the daemon when the plugin declares Mode-C for this op.
func (a *App) dispatchPluginVerb(ctx context.Context, verb string, args []string) error {
	if a.PluginHost == nil {
		return fmt.Errorf("plugin installed but %q is still unavailable. Rerun the command", verb)
	}
	route := a.PluginHost.CommandRoute(verb)
	if route == nil {
		return fmt.Errorf("plugin installed but %q is still unavailable. Rerun the command", verb)
	}
	if pool := a.DaemonPool(); pool != nil &&
		pluginhost.SupportsModeCDispatch(route.Plugin.Name(), verb) &&
		route.Plugin.Manifest.Daemon != nil {
		return pluginhost.DispatchViaDaemon(ctx, pool, route.Plugin, verb, args)
	}
	return pluginhost.ExecPluginCommand(ctx, route, args)
}

// missingPluginOffer is the interactive header shown above the prompt.
func missingPluginOffer(hint PluginHint, verb string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n`kapi %s` comes from the %s plugin, which is not installed.\n", verb, hint.Plugin)
	fmt.Fprintf(&b, "This project's recipe declares a `%s:` block, so it expects it.\n\n", hint.RecipeKey)
	fmt.Fprintf(&b, "  Install it:  kapi plugin install %s\n", hint.Plugin)
	fmt.Fprintf(&b, "               brew install %s\n\n", hint.Formula)
	return b.String()
}

// missingPluginError is the non-interactive outcome: the same guidance, as an
// error, so it travels through the normal error path (and the --json error
// envelope) instead of being printed behind the caller's back.
func missingPluginError(hint PluginHint, verb string) error {
	return fmt.Errorf("`kapi %s` comes from the %s plugin, which is not installed "+
		"(this project's recipe declares a `%s:` block); install it with `kapi plugin install %s` or `brew install %s`",
		verb, hint.Plugin, hint.RecipeKey, hint.Plugin, hint.Formula)
}
