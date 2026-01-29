package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pluginsCmd = &cobra.Command{
	Use:   "plugins",
	Short: "Manage plugins",
}

var pluginsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Installed plugins:")
		fmt.Println("  (none)")
		fmt.Println()
		fmt.Println("Use 'kapi plugins install <name>@<version>' to install a plugin.")
	},
}

var pluginsInstallCmd = &cobra.Command{
	Use:   "install [name@version]",
	Short: "Install a plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		plugin := args[0]
		if !quiet {
			fmt.Printf("Installing plugin: %s\n", plugin)
		}
		// Plugin installation is a Phase 3 feature.
		// For now, this is a placeholder.
		fmt.Printf("Plugin system not yet configured. Use --plugin-dir to specify a plugin directory.\n")
		return nil
	},
}

var pluginsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update all installed plugins",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("All plugins are up to date.")
	},
}

func init() {
	pluginsCmd.AddCommand(pluginsListCmd)
	pluginsCmd.AddCommand(pluginsInstallCmd)
	pluginsCmd.AddCommand(pluginsUpdateCmd)
	rootCmd.AddCommand(pluginsCmd)
}
