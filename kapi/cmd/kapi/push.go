package main

import (
	"fmt"

	"github.com/gokapi/gokapi/platform/connector"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var (
	pushForce  bool
	pushDryRun bool
)

var pushCmd = &cobra.Command{
	Use:   "push [paths...]",
	Short: "Upload local changes to the server",
	Long: `Upload local changes to the server.

Only changed blocks are sent. Runs pre-push hooks if configured.`,
	RunE: runPush,
}

func runPush(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return err
	}

	conn, err := project.NewSourceConnector(proj, formatReg)
	if err != nil {
		return err
	}
	defer conn.Close()

	result, err := conn.Push(cmd.Context(), connector.PushOptions{
		Paths:  args,
		Force:  pushForce,
		DryRun: pushDryRun,
	})
	if err != nil {
		return err
	}

	if pushDryRun {
		fmt.Printf("Would push %d blocks, %d words (scanned %d files)\n", result.BlocksPushed, result.WordCount, result.FilesScanned)
		return nil
	}

	if result.BlocksPushed == 0 {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("Pushed %d blocks, %d words (scanned %d files)\n", result.BlocksPushed, result.WordCount, result.FilesScanned)
	return nil
}

func init() {
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Re-upload everything, even unchanged blocks")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "Show what would be uploaded without sending")
	rootCmd.AddCommand(pushCmd)
}
