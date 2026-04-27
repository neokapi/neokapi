package main

import (
	"github.com/neokapi/neokapi/cli"
	cliconfig "github.com/neokapi/neokapi/cli/config"
	"github.com/spf13/cobra"
)

var app = &cli.App{}

var rootCmd = &cobra.Command{
	Use:           "bowrain",
	Short:         "Manage localization projects",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `bowrain manages localization projects, syncing content with Bowrain Server.

Initialize a kapi project in your repository, then push/pull translations,
run quality checks, and manage terminology.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		app.Config = newBowrainAppConfig()
		// TODO: registries are not yet represented on the framework's
		// KapiProject recipe; once they land there, restore the
		// recipe-level RegistryResolver hook here.
		app.RegistryResolver = func() []cliconfig.RegistryEntry { return nil }
		app.Init()
		// Plugins (schema, commands, mcp) installed AppInitializers via
		// init(); apply them after Init has set up the registries and
		// config so they can read everything they need from app.
		cli.ApplyAppInitializers(app)
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		app.Shutdown()
	},
}

// newBowrainAppConfig creates a config reader for bowrain that layers
// bowrain-specific config (~/.config/bowrain/bowrain.yaml) on top of the
// shared kapi config. Bowrain-specific settings like server.url are read
// from the bowrain config; shared settings (plugins, formats, flow) come
// from the kapi config.
func newBowrainAppConfig() *cliconfig.AppConfig {
	return cliconfig.NewOverlayAppConfig("bowrain", func(cfg *cliconfig.AppConfig) {
		cfg.Viper().SetDefault("server.url", "http://localhost:8080")
		_ = cfg.Viper().BindEnv("server.url", "BOWRAIN_SERVER_URL")
	})
}

func init() {
	// Populate tool + format registries up front so NewToolCommands can
	// see every built-in tool before cobra's init runs. PersistentPreRun
	// calls Init() later to do the flag-dependent work (plugins,
	// credentials, config load).
	app.InitRegistries()

	app.AddPersistentFlags(rootCmd)
	app.AddCommandGroups(rootCmd)

	// Primary commands. The bowrain plugin's commands package installs
	// app.FallbackRunE (project flows) and app.ExtraFlows (.kapi/flows/
	// listing) via an AppInitializer; the run / flows commands read those
	// fields at run time, so we don't need to pass them through opts here.
	runCmd := app.NewRunCmd(cli.RunCmdOptions{})
	runCmd.GroupID = "processing"
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(app.NewExtractCmd(cli.ExtractCmdOptions{}))
	rootCmd.AddCommand(app.NewMergeCmd(cli.MergeCmdOptions{}))

	// Management commands.
	rootCmd.AddCommand(app.NewFlowsCmd(cli.FlowCmdOptions{}))
	rootCmd.AddCommand(app.NewToolsCmd())
	rootCmd.AddCommand(app.NewFormatsCmd())
	rootCmd.AddCommand(app.NewPluginsCmd())
	rootCmd.AddCommand(app.NewRegistryCmd())
	rootCmd.AddCommand(app.NewPresetsCmd())
	rootCmd.AddCommand(app.NewTermbaseCmd())
	rootCmd.AddCommand(app.NewTMCmd())
	rootCmd.AddCommand(app.NewCredentialsCmd())
	rootCmd.AddCommand(app.NewVersionCmd("bowrain"))
	rootCmd.AddCommand(app.NewCompletionCmd())

	// Top-level tool commands (declarative opt-in via BuiltinToolCommands).
	for _, cmd := range app.NewToolCommands() {
		rootCmd.AddCommand(cmd)
	}

	mcpCmd := app.NewMCPCmd("bowrain")
	mcpCmd.GroupID = "processing"
	rootCmd.AddCommand(mcpCmd)

	// Plugin commands (init, push, pull, status, ls, add, rm, sync,
	// auth, config, diff, stream, serve, ui) get added by the bowrain
	// plugin's commands package via cli.ApplyCommandFactories.
	cli.ApplyCommandFactories(rootCmd, app)
}
