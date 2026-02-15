package main

import (
	"fmt"
	"path/filepath"

	"github.com/gokapi/gokapi/core/kapiproject"
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
	proj, err := kapiproject.FindProject("")
	if err != nil {
		return err
	}
	fmt.Printf("Project root: %s\n", proj.Root)
	fmt.Printf("Config:       %s\n", filepath.Join(proj.KapiDir, "config.yaml"))

	conn, err := kapiproject.NewSourceConnector(proj, formatReg)
	if err != nil {
		// No server configured — show local info only.
		fmt.Println("\nSync status requires a Bowrain server connection.")
		fmt.Printf("  Configure server in %s\n", filepath.Join(proj.KapiDir, "config.yaml"))
		return nil
	}
	defer conn.Close()

	status, err := conn.Status(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("\nLocal blocks: %d\n", status.ItemCount)

	if status.PendingPush > 0 {
		fmt.Printf("Pending push: %d blocks changed locally\n", status.PendingPush)
	}
	if status.PendingPull > 0 {
		fmt.Printf("Pending pull: %d remote changes available\n", status.PendingPull)
	} else if status.PendingPull < 0 {
		fmt.Println("Pending pull: remote changes available (count unknown)")
	}
	if status.PendingPush == 0 && status.PendingPull == 0 {
		fmt.Println("Up to date.")
	}

	if !status.LastSync.IsZero() {
		fmt.Printf("Last sync:    %s\n", status.LastSync.Format("2006-01-02 15:04:05 UTC"))
	}

	if len(status.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, e := range status.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
