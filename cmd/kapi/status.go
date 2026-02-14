package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gokapi/gokapi/core/kapiproject"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync state between local files and Bowrain Server",
	Long: `Display the sync state showing modified local files, remote changes,
and conflicts between local and remote versions.

Similar to 'git status' but for localization files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Find project
		project, err := kapiproject.FindProject("")
		if err != nil {
			return fmt.Errorf("find project: %w (run 'kapi init' to create a project)", err)
		}

		// Load state
		state, err := project.LoadState(ctx)
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}

		// Display sync status
		fmt.Printf("Project: %s\n", project.Config.Project.Name)
		fmt.Printf("Root: %s\n\n", project.Root)

		if !state.LastPull.IsZero() {
			fmt.Printf("Last pull: %s\n", state.LastPull.Format(time.RFC3339))
		} else {
			fmt.Println("Last pull: never")
		}

		if !state.LastPush.IsZero() {
			fmt.Printf("Last push: %s\n", state.LastPush.Format(time.RFC3339))
		} else {
			fmt.Println("Last push: never")
		}

		fmt.Println()

		// Check for modified local files
		modifiedFiles := []string{}
		for path, fileState := range state.Files {
			fullPath := project.ResolvePath(path)
			if st, err := os.Stat(fullPath); err == nil {
				// File exists, check if modified
				if st.ModTime().After(fileState.Modified) {
					modifiedFiles = append(modifiedFiles, path)
				}
			}
		}

		if len(modifiedFiles) > 0 {
			fmt.Println("Modified local files:")
			for _, path := range modifiedFiles {
				fmt.Printf("  M %s\n", path)
			}
		} else {
			fmt.Println("No local changes")
		}

		// Server status (if configured)
		if project.Config.Server != nil {
			fmt.Printf("\nRemote: %s\n", project.Config.Server.URL)
			fmt.Printf("Project ID: %s\n", project.Config.Server.ProjectID)
			fmt.Println("\nRemote changes: (not yet implemented)")
			fmt.Println("Run 'kapi pull --dry-run' to see remote changes")
		} else {
			fmt.Println("\nNo server configured")
			fmt.Println("Run 'kapi init --server <URL> --project <ID>' to connect")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
