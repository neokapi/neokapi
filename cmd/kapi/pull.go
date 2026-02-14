package main

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/kapiproject"
	"github.com/spf13/cobra"
)

var (
	pullForce  bool
	pullDryRun bool
)

var pullCmd = &cobra.Command{
	Use:   "pull [paths...]",
	Short: "Pull changes from Bowrain Server",
	Long: `Fetch changes from Bowrain Server and update local files.

Only changed blocks are transferred (incremental sync using content hashing).
Runs post-pull hooks if configured in .kapi/config.yaml.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Find project
		project, err := kapiproject.FindProject("")
		if err != nil {
			return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
		}

		// Check server configuration
		if project.Config.Server == nil {
			return fmt.Errorf("no server configured (run 'kapi init --server <URL> --project <ID>')")
		}

		// Load state
		state, err := project.LoadState(ctx)
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}

		fmt.Printf("Pulling from: %s\n", project.Config.Server.URL)
		fmt.Printf("Project: %s\n", project.Config.Server.ProjectID)

		if pullDryRun {
			fmt.Println("\n[DRY RUN] No files will be modified")
		}

		// TODO: Implement full pull logic:
		// 1. Verify auth token
		// 2. Call POST /api/v1/workspaces/:ws/projects/:id/pull
		//    - Send local state (hashes, timestamps)
		//    - Receive only changed blocks
		// 3. Check for conflicts (both local and remote changed)
		// 4. Write blocks to local files via FormatRegistry
		// 5. Run post-pull hooks (if configured)
		// 6. Update .state.json

		fmt.Println("\nPull implementation: Not yet implemented")
		fmt.Println()
		fmt.Println("Full implementation requires:")
		fmt.Println("  - Server API endpoints (/api/v1/.../pull)")
		fmt.Println("  - Content hash computation")
		fmt.Println("  - FormatRegistry integration for writing files")
		fmt.Println("  - Hook execution framework")
		fmt.Println()
		fmt.Println("Current state loaded from:", project.KapiDir)
		fmt.Printf("Tracked files: %d\n", len(state.Files))

		return nil
	},
}

func init() {
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Overwrite local changes")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "Show what would be pulled")

	rootCmd.AddCommand(pullCmd)
}
