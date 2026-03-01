package main

import (
	"fmt"

	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show differences between local and remote",
	Long: `Display differences between local files and remote project.

Shows which blocks have changed locally or remotely since the last sync.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Find project
		proj, err := project.FindProject("")
		if err != nil {
			return fmt.Errorf("find project: %w (run 'brain init' to create a project)", err)
		}

		fmt.Printf("Project: %s\n\n", proj.Config.Project.Name)

		if proj.Config.Server == nil {
			fmt.Println("No server configured")
			fmt.Println("Run 'brain init' to connect to a server")
			return nil
		}

		fmt.Println("Diff implementation: Not yet implemented")
		fmt.Println()
		fmt.Println("This command will show:")
		fmt.Println("  - Blocks changed locally")
		fmt.Println("  - Blocks changed remotely")
		fmt.Println("  - Conflicts (both changed)")
		fmt.Println()
		fmt.Println("For now, use 'brain status' to see basic sync state")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
