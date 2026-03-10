package main

import (
	"path/filepath"

	"github.com/gokapi/gokapi/bowrain-cli/cmd/bowrain/output"
	"github.com/gokapi/gokapi/platform/project"
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
			ConfigDir: filepath.Join(proj.ConfigDir, "config.yaml"),
		},
	}

	conn, err := project.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		// No server configured — show local info only.
		return output.Print(cmd, out)
	}
	defer conn.Close()

	status, err := conn.Status(cmd.Context())
	if err != nil {
		return err
	}

	out.Project.Server = proj.Config.ServerURL()
	out.Project.ProjectID = proj.Config.ProjectID()
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
	rootCmd.AddCommand(statusCmd)
}
