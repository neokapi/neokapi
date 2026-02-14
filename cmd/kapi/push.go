package main

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/kapiproject"
	"github.com/spf13/cobra"
)

var (
	pushForce   bool
	pushDryRun  bool
	pushMessage string
	pushNoHooks bool
)

var pushCmd = &cobra.Command{
	Use:   "push [paths...]",
	Short: "Push local changes to Bowrain Server",
	Long: `Send local file changes to Bowrain Server.

Only changed blocks are transferred (incremental sync using content hashing).
Runs pre-push hooks if configured in .kapi/config.yaml (unless --no-hooks).`,
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

		fmt.Printf("Pushing to: %s\n", project.Config.Server.URL)
		fmt.Printf("Project: %s\n", project.Config.Server.ProjectID)

		if pushDryRun {
			fmt.Println("\n[DRY RUN] No files will be uploaded")
		}

		// Check for pre-push hooks
		if !pushNoHooks {
			if hooks, ok := project.Config.Hooks["pre-push"]; ok && len(hooks) > 0 {
				fmt.Printf("\nRunning pre-push hooks: %v\n", hooks)
				// TODO: Execute hooks
				fmt.Println("(Hook execution not yet implemented)")
			}
		}

		// TODO: Implement full push logic:
		// 1. Read local files via FormatRegistry
		// 2. Compute block hashes
		// 3. Compare with .state.json → identify changed blocks
		// 4. Verify auth token
		// 5. Call POST /api/v1/workspaces/:ws/projects/:id/push
		//    - Send changed blocks + item mappings + message
		//    - Server may reject if quality gates fail
		// 6. Update .state.json

		fmt.Println("\nPush implementation: Not yet implemented")
		fmt.Println()
		fmt.Println("Full implementation requires:")
		fmt.Println("  - FormatRegistry integration for reading files")
		fmt.Println("  - Block hash computation")
		fmt.Println("  - Server API endpoints (/api/v1/.../push)")
		fmt.Println("  - Hook execution framework")
		fmt.Println()
		fmt.Println("Current state loaded from:", project.KapiDir)
		fmt.Printf("Tracked files: %d\n", len(state.Files))

		if pushMessage != "" {
			fmt.Printf("Commit message: %s\n", pushMessage)
		}

		return nil
	},
}

func init() {
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Bypass quality gates")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "Show what would be pushed")
	pushCmd.Flags().StringVarP(&pushMessage, "message", "m", "", "Commit message")
	pushCmd.Flags().BoolVar(&pushNoHooks, "no-hooks", false, "Skip pre-push hooks")

	rootCmd.AddCommand(pushCmd)
}
