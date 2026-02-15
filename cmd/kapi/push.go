package main

import (
	"fmt"

	"github.com/gokapi/gokapi/core/connector"
	"github.com/gokapi/gokapi/core/kapiproject"
	"github.com/spf13/cobra"
)

var (
	pushForce  bool
	pushDryRun bool
)

var pushCmd = &cobra.Command{
	Use:   "push [paths...]",
	Short: "Push local changes to Bowrain Server",
	Long: `Send local file changes to Bowrain Server.

Only changed blocks are transferred (incremental sync using content hashing).
Runs pre-push hooks if configured in .kapi/config.yaml (unless --no-hooks).`,
	RunE: runPush,
}

func runPush(cmd *cobra.Command, args []string) error {
	proj, err := kapiproject.FindProject("")
	if err != nil {
		return err
	}

	conn, err := kapiproject.NewSourceConnector(proj, formatReg)
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
		fmt.Printf("Would push %d blocks (scanned %d files)\n", result.BlocksPushed, result.FilesScanned)
		return nil
	}

	if result.BlocksPushed == 0 {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("Pushed %d blocks (scanned %d files)\n", result.BlocksPushed, result.FilesScanned)
	if result.ChunkCount > 1 {
		fmt.Printf("  (sent in %d batches)\n", result.ChunkCount)
	}
	return nil
}

func init() {
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Push all blocks, ignoring sync cache")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "Report what would be pushed without sending")
	rootCmd.AddCommand(pushCmd)
}
