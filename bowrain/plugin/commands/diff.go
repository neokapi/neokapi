package commands

import (
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/cli"
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
			return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
		}

		fmt.Printf("Project: %s\n\n", filepath.Base(proj.Root))

		if !proj.Recipe.HasServer() {
			fmt.Println("No server configured")
			fmt.Println("Run 'kapi init' to connect to a server")
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
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(diffCmd) })
}
