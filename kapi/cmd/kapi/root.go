package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/cli/pluginattach"
	"github.com/neokapi/neokapi/cli/selfupdate"
	"github.com/neokapi/neokapi/core/channel"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/config"
	"github.com/spf13/cobra"
)

// updateChannel resolves the release track the background notifier watches — the
// persisted update.channel (KAPI_UPDATE_CHANNEL), falling back to the channel
// inferred from this build (a prerelease defaults to beta).
func updateChannel() string {
	if app.Config == nil {
		return channel.Resolve()
	}
	return app.Config.UpdateChannel()
}

var app = &cli.App{}

var rootCmd = &cobra.Command{
	Use:           "kapi",
	Short:         cli.KapiRootShort,
	Version:       version.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	Long:          cli.KapiRootLong,
	// Bare `kapi` inside a project shows the status summary plus a `kapi up`
	// hint; outside a project it falls back to the standard help. Unknown
	// subcommands still error (cobra's legacy args validation runs first),
	// and --help output is untouched.
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.RootRunE(app, cmd, args)
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		app.Config = config.NewAppConfig()
		if err := app.Init(); err != nil {
			return err
		}
		// Plugins (e.g. bowrain) register App initializers at init().
		// Apply them after Init has set up registries and config.
		cli.ApplyAppInitializers(app)
		// Make beta-channel membership sticky: a fresh prerelease build pins
		// update.channel=beta so it stays on the fast track after later updating
		// to a final release. No-op once pinned or for stable builds.
		app.Config.EnsureChannelPinned("kapi")
		// Kick off a detached, time-bounded check for a newer kapi release. It
		// only warms the on-disk cache; the notice (if any) is rendered in
		// PostRun. No-ops in CI / non-TTY / when opted out.
		selfupdate.StartBackgroundRefresh(updateChannel())
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		app.Shutdown()
		// Surface an "update available" line (cache-only, never blocks). Skip
		// for `kapi update`, which already speaks to the user about versions.
		if cmd.Name() != "update" {
			selfupdate.RenderNotice(os.Stderr, updateChannel())
		}
	},
}

func init() {
	// Populate tool + format registries up front so NewToolCommands can
	// see every built-in tool before cobra's init runs. PersistentPreRun
	// calls Init() later to do the flag-dependent work (gRPC plugins,
	// credentials, config load).
	app.InitRegistries()

	// Discover manifest-driven plugins early so their commands wire
	// into the cobra tree before Execute parses argv.
	app.InitPluginHost()

	cli.AddPersistentFlags(app, rootCmd)
	cli.AddCommandGroups(app, rootCmd)

	// Built-in command set, shared with the cli/i18n help-string generator
	// (cli.KapiCommandSet is the single source of truth for what `kapi`
	// exposes, so the localization inventory can never drift from the
	// binary).
	//
	// Built-ins always register: installing a plugin must never change what
	// an existing verb means (the no-shadowing rule — plugins extend kapi
	// the way gh extends git). A plugin that needs to participate in a core
	// verb does so through a contribution (bowrain's init --server), a
	// hidden plumbing command the built-in dispatches (server-status under
	// kapi status, server-up under kapi up), or its plugin group
	// (kapi bowrain config). pluginattach enforces this: a plugin command
	// colliding with a built-in attaches group-only.
	for _, cmd := range cli.KapiCommandSet(app) {
		rootCmd.AddCommand(cmd)
	}

	// Snapshot the built-in verb set BEFORE plugin commands attach: the
	// opt-out telemetry reporter (epic 018, workstream G) only ever names
	// built-in verbs — plugin-dispatched or unknown verbs report as
	// "plugin"/"other". The reporter runs on cli.Run's exit path after the
	// command completes, and flushes best-effort within 100 ms.
	builtinVerbs := make(map[string]bool, len(rootCmd.Commands()))
	for _, cmd := range rootCmd.Commands() {
		builtinVerbs[cmd.Name()] = true
	}
	cli.SetCommandReporter(cli.NewTelemetryReporter(app, builtinVerbs))

	// Plugins (e.g. bowrain via blank import in main.go) register their
	// commands at init() time; wire them in after the built-in command
	// tree is constructed.
	cli.ApplyCommandFactories(rootCmd, app)

	// Manifest-driven plugins discovered by InitPluginHost contribute
	// their Mode-A commands here. Conflicts with built-ins or other
	// plugins are reported on stderr and the conflicting capability
	// is omitted from dispatch.
	//
	// When a plugin declares a Mode-C daemon block AND a
	// SourceConnectorDispatcher is registered for the plugin's name,
	// matching commands route through the daemon pool instead of
	// spawning a fresh subprocess per invocation.
	pluginattach.AttachCommandsWithOptions(rootCmd, app.PluginHost, pluginattach.AttachOptions{
		OnConflict: func(msg string) {
			if !app.Quiet {
				fmt.Fprintln(os.Stderr, "Warning: "+msg)
			}
		},
		DaemonPool: app.DaemonPool(),
	})

	// Plugin contributions augment built-in commands (e.g. bowrain extends
	// `kapi init` to connect a project to a server). Wire these after the
	// built-in + plugin command trees are in place so the target commands exist.
	pluginattach.AttachContributions(rootCmd, app.PluginHost, func(msg string) {
		if !app.Quiet {
			fmt.Fprintln(os.Stderr, "Warning: "+msg)
		}
	})

	// Localize command help in place. This must happen at construction
	// time: cobra renders --help before any PreRun hook, so the --lang
	// flag cannot apply — help honors KAPI_LANG / config / POSIX env (see
	// cli.HelpTranslator). Misses keep the English source.
	cli.LocalizeCommandHelp(rootCmd, cli.HelpTranslator())

	// Colorize --help / usage through host/output's Theme (same palette and
	// --color/NO_COLOR/isatty handling as the rest of the CLI). Must run after
	// localization: the template reads Short/Long/Example at render time.
	cli.SetupStyledHelp(rootCmd)
}
