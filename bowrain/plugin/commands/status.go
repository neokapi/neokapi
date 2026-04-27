package commands

import (
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/bowrain/core/project"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project sync status",
	Long:  `Show what has changed locally and on the server since the last sync.`,
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return err
	}

	out := output.StatusOutput{
		Project: output.ProjectInfo{
			Root:      proj.Root,
			ConfigDir: proj.RecipePath(),
		},
	}

	conn, err := bconn.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		// No server configured — show local info only.
		return output.Print(cmd, out)
	}
	defer conn.Close()

	status, err := conn.Status(cmd.Context())
	if err != nil {
		return err
	}

	out.Project.Server = proj.Recipe.Server.ServerURL()
	out.Project.ProjectID = proj.Recipe.Server.ProjectID()
	out.Stream = conn.Stream()
	out.ItemCount = status.ItemCount
	out.FileCount = status.FileCount
	out.WordCount = status.WordCount
	out.PendingPush = status.PendingPush
	out.PendingPull = status.PendingPull
	out.UpToDate = status.PendingPush == 0 && status.PendingPull == 0
	if !status.LastSync.IsZero() {
		out.LastSync = &status.LastSync
	}
	out.Errors = status.Errors

	return output.Print(cmd, out)
}

func init() {
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(statusCmd) })
}
