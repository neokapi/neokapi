package main

import (
	"path/filepath"

	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync state between local files and Bowrain Server",
	Long: `Display the sync state showing modified local files, remote changes,
and conflicts between local and remote versions.

Similar to 'git status' but for localization files.`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return err
	}

	out := output.StatusOutput{
		Project: output.ProjectInfo{
			Root:      proj.Root,
			ConfigDir: filepath.Join(proj.KapiDir, "config.yaml"),
		},
	}

	conn, err := project.NewSourceConnector(proj, formatReg)
	if err != nil {
		// No server configured — show local info only.
		return output.Print(cmd, out)
	}
	defer conn.Close()

	status, err := conn.Status(cmd.Context())
	if err != nil {
		return err
	}

	out.Project.Server = proj.Config.Server.URL
	out.Project.ProjectID = proj.Config.Server.ProjectID
	out.ItemCount = status.ItemCount
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
