package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/cli/selfupdate"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

// updateChannel resolves the release track the background notifier watches —
// the configured update.channel (KAPI_UPDATE_CHANNEL), defaulting to stable.
func updateChannel() string {
	if app.Config == nil {
		return config.DefaultUpdateChannel
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		app.Config = config.NewAppConfig()
		if err := app.Init(); err != nil {
			return err
		}
		// Plugins (e.g. bowrain) register App initializers at init().
		// Apply them after Init has set up registries and config.
		cli.ApplyAppInitializers(app)
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

	app.AddPersistentFlags(rootCmd)
	app.AddCommandGroups(rootCmd)

	// Built-in command set, shared with the cli/i18n help-string generator
	// (cli.KapiCommandSet is the single source of truth for what `kapi`
	// exposes, so the localization inventory can never drift from the
	// binary).
	for _, cmd := range app.KapiCommandSet() {
		rootCmd.AddCommand(cmd)
	}

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
	pluginhost.AttachCommandsWithOptions(rootCmd, app.PluginHost, pluginhost.AttachOptions{
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
	pluginhost.AttachContributions(rootCmd, app.PluginHost, func(msg string) {
		if !app.Quiet {
			fmt.Fprintln(os.Stderr, "Warning: "+msg)
		}
	})

	// Localize command help in place. This must happen at construction
	// time: cobra renders --help before any PreRun hook, so the --lang
	// flag cannot apply — help honors KAPI_LANG / config / POSIX env (see
	// cli.HelpTranslator). Misses keep the English source.
	cli.LocalizeCommandHelp(rootCmd, cli.HelpTranslator())
}
