package main

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/kapiproject"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show differences between local and remote",
	Long: `Display differences between local files and remote project.

Shows which blocks have changed locally or remotely since the last sync.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Find project
		project, err := kapiproject.FindProject("")
		if err != nil {
			return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
		}

		// Load state
		_, err = project.LoadState(ctx)
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}

		fmt.Printf("Project: %s\n\n", project.Config.Project.Name)

		if project.Config.Server == nil {
			fmt.Println("No server configured")
			fmt.Println("Run 'kapi init --server <URL> --project <ID>' to connect")
			return nil
		}

		fmt.Println("Diff implementation: Not yet implemented")
		fmt.Println()
		fmt.Println("This command will show:")
		fmt.Println("  - Blocks changed locally")
		fmt.Println("  - Blocks changed remotely")
		fmt.Println("  - Conflicts (both changed)")
		fmt.Println()
		fmt.Println("For now, use 'kapi status' to see basic sync state")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
