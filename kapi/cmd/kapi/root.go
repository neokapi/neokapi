package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
)

var app = &cli.App{}

var rootCmd = &cobra.Command{
	Use:           "kapi",
	Short:         "A localization and translation toolkit",
	Version:       version.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `kapi helps you manage multilingual content — convert document formats,
translate with AI, and run quality checks across a wide range of file types.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		app.Config = config.NewAppConfig()
		if err := app.Init(); err != nil {
			return err
		}
		// Plugins (e.g. bowrain) register App initializers at init().
		// Apply them after Init has set up registries and config.
		cli.ApplyAppInitializers(app)
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		app.Shutdown()
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

	// Primary commands.
	runCmd := app.NewRunCmd(cli.RunCmdOptions{})
	runCmd.GroupID = "processing"
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(app.NewExtractCmd(cli.ExtractCmdOptions{}))
	rootCmd.AddCommand(app.NewMergeCmd(cli.MergeCmdOptions{}))

	// .klz project snapshot hand-off (AD-025 §5): pack the working state
	// into a portable .klz and rehydrate it elsewhere. Resume across runs is
	// the persistent block-store cache, not a stepping verb family.
	rootCmd.AddCommand(app.NewPackCmd())
	rootCmd.AddCommand(app.NewUnpackCmd())
	rootCmd.AddCommand(app.NewInfoCmd())

	// Toolbox: format-aware cat / grep / sed. Registered as hidden, flag-detached
	// proxies that delegate to the same standalone commands the multi-call
	// binaries kcat / kgrep / ksed run (see main.go busybox dispatch). Hidden so
	// `kapi --help` steers users to the dedicated k-commands.
	for _, c := range app.NewToolboxProxies() {
		rootCmd.AddCommand(c)
	}
	rootCmd.AddCommand(app.NewVerifyCmd())
	rootCmd.AddCommand(app.NewCheckCmd())
	rootCmd.AddCommand(app.NewHookCmd())
	rootCmd.AddCommand(app.NewInitCmd())
	rootCmd.AddCommand(app.NewAddCmd())
	rootCmd.AddCommand(app.NewRmCmd())
	rootCmd.AddCommand(app.NewLsCmd())

	// Management commands.
	rootCmd.AddCommand(app.NewFlowsCmd(cli.FlowCmdOptions{}))
	rootCmd.AddCommand(app.NewToolsCmd())
	rootCmd.AddCommand(app.NewFormatsCmd())
	rootCmd.AddCommand(app.NewPluginCmd())
	rootCmd.AddCommand(app.NewRegistryCmd())
	rootCmd.AddCommand(app.NewPresetsCmd())
	rootCmd.AddCommand(app.NewTermbaseCmd())
	rootCmd.AddCommand(app.NewTMCmd())
	rootCmd.AddCommand(app.NewBrandCmd())
	rootCmd.AddCommand(app.NewSkillsCmd())
	rootCmd.AddCommand(app.NewCredentialsCmd())
	rootCmd.AddCommand(app.NewVersionCmd("kapi"))
	rootCmd.AddCommand(app.NewCompletionCmd())

	// Top-level tool commands (declarative opt-in via BuiltinToolCommands).
	for _, cmd := range app.NewToolCommands() {
		rootCmd.AddCommand(cmd)
	}

	mcpCmd := app.NewMCPCmd("kapi")
	mcpCmd.GroupID = "processing"
	rootCmd.AddCommand(mcpCmd)

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
}
