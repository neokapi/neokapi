package main

import (
	"context"

	"github.com/gokapi/gokapi/bowrain-cli/cmd/bowrain/output"
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

// PushResult holds the structured result of a push operation.
type PushResult struct {
	BlocksPushed int
	WordCount    int
	FilesScanned int
	PushID       string
	DryRun       bool
	UpToDate     bool
}

// doPush executes the core push logic and returns structured results.
func doPush(ctx context.Context, opts connector.PushOptions, args []string) (*PushResult, *project.BrainSourceConnector, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, nil, err
	}

	conn, err := project.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		return nil, nil, err
	}

	result, err := conn.Push(ctx, connector.PushOptions{
		Paths:  args,
		Force:  opts.Force,
		DryRun: opts.DryRun,
	})
	if err != nil {
		conn.Close()
		return nil, nil, err
	}

	pr := &PushResult{
		BlocksPushed: result.BlocksPushed,
		WordCount:    result.WordCount,
		FilesScanned: result.FilesScanned,
		PushID:       result.PushID,
	}
	if opts.DryRun {
		pr.DryRun = true
	} else if result.BlocksPushed == 0 {
		pr.UpToDate = true
	}

	return pr, conn, nil
}

func runPush(cmd *cobra.Command, args []string) error {
	pr, conn, err := doPush(cmd.Context(), connector.PushOptions{
		Force:  pushForce,
		DryRun: pushDryRun,
	}, args)
	if err != nil {
		return err
	}
	defer conn.Close()

	out := output.PushOutput{
		BlocksPushed: pr.BlocksPushed,
		WordCount:    pr.WordCount,
		FilesScanned: pr.FilesScanned,
		DryRun:       pr.DryRun,
		UpToDate:     pr.UpToDate,
	}

	return output.Print(cmd, out)
}

func init() {
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Re-upload everything, even unchanged blocks")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "Show what would be uploaded without sending")
	rootCmd.AddCommand(pushCmd)
}
