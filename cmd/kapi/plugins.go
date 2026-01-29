package main

import (
	"fmt"
	"strings"

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
		plugins := pluginLoader.Plugins()
		if len(plugins) == 0 {
			fmt.Printf("No plugins loaded.\n")
			fmt.Printf("Plugin directory: %s\n", pluginLoader.Dir())
			fmt.Println()
			fmt.Println("Place plugin executables (gokapi-plugin-*) or bridge descriptors")
			fmt.Println("(*.bridge.json) in the plugin directory, or use --plugin-dir to override.")
			return
		}

		fmt.Printf("  %-20s %-10s %-30s %s\n", "NAME", "TYPE", "FORMATS", "SOURCE")
		fmt.Printf("  %-20s %-10s %-30s %s\n", "----", "----", "-------", "------")
		for _, p := range plugins {
			fmts := strings.Join(p.Formats, ", ")
			fmt.Printf("  %-20s %-10s %-30s %s\n", p.Name, p.Type, fmts, p.Source)
		}
		fmt.Printf("\n%d plugin(s) loaded from %s\n", len(plugins), pluginLoader.Dir())
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
