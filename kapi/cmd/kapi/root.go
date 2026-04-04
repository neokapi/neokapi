package main

import (
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/cli/config"
	"github.com/spf13/cobra"
)

var app = &cli.App{}

var rootCmd = &cobra.Command{
	Use:           "kapi",
	Short:         "A localization and translation toolkit",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `kapi helps you manage multilingual content — convert document formats,
translate with AI, and run quality checks across a wide range of file types.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		app.Config = config.NewAppConfig()
		app.Init()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		app.Shutdown()
	},
}

func init() {
	app.AddPersistentFlags(rootCmd)
	app.AddCommandGroups(rootCmd)

	// Primary commands.
	runCmd := app.NewRunCmd(cli.RunCmdOptions{})
	runCmd.GroupID = "processing"
	rootCmd.AddCommand(runCmd)

	// Management commands.
	rootCmd.AddCommand(app.NewFlowsCmd(cli.FlowCmdOptions{}))
	rootCmd.AddCommand(app.NewToolsCmd())
	rootCmd.AddCommand(app.NewFormatsCmd())
	rootCmd.AddCommand(app.NewPluginsCmd())
	rootCmd.AddCommand(app.NewRegistryCmd())
	rootCmd.AddCommand(app.NewPresetsCmd())
	rootCmd.AddCommand(app.NewTermbaseCmd())
	rootCmd.AddCommand(app.NewTMCmd())
	rootCmd.AddCommand(app.NewVersionCmd("kapi"))
	rootCmd.AddCommand(app.NewCompletionCmd())

	// Top-level tool commands (declarative opt-in via BuiltinToolCommands).
	for _, cmd := range app.NewToolCommands() {
		rootCmd.AddCommand(cmd)
	}

	mcpCmd := newMCPCmd()
	mcpCmd.GroupID = "processing"
	rootCmd.AddCommand(mcpCmd)
}
