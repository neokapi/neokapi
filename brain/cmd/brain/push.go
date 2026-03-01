package main

import (
	"github.com/gokapi/gokapi/brain/cmd/brain/output"
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

	conn, err := project.NewSourceConnector(proj, app.FormatReg)
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

	out := output.PushOutput{
		BlocksPushed: result.BlocksPushed,
		WordCount:    result.WordCount,
		FilesScanned: result.FilesScanned,
	}
	if pushDryRun {
		out.DryRun = true
	} else if result.BlocksPushed == 0 {
		out.UpToDate = true
	}

	return output.Print(cmd, out)
}

func init() {
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Re-upload everything, even unchanged blocks")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "Show what would be uploaded without sending")
	rootCmd.AddCommand(pushCmd)
}
