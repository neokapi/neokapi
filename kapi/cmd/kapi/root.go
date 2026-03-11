package main

import (
	"github.com/gokapi/gokapi/cli"
	"github.com/gokapi/gokapi/cli/config"
	"github.com/spf13/cobra"
)

var app = &cli.App{}

var rootCmd = &cobra.Command{
	Use:          "kapi",
	Short:        "A localization and translation toolkit",
	SilenceUsage: true,
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

	rootCmd.AddCommand(app.NewFlowCmd(cli.FlowCmdOptions{}))
	rootCmd.AddCommand(app.NewFormatsCmd())
	rootCmd.AddCommand(app.NewPluginsCmd())
	rootCmd.AddCommand(app.NewRegistryCmd())
	rootCmd.AddCommand(app.NewToolsCmd())
	rootCmd.AddCommand(app.NewPresetsCmd())
	rootCmd.AddCommand(app.NewTermbaseCmd())
	rootCmd.AddCommand(app.NewTMCmd())
	rootCmd.AddCommand(app.NewVersionCmd("kapi"))

	for _, cmd := range app.NewToolCommands() {
		rootCmd.AddCommand(cmd)
	}

	rootCmd.AddCommand(newMCPCmd())
}
