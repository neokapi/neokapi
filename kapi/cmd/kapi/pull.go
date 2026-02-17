package main

import (
	"fmt"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/platform/connector"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var (
	pullLocales []string
	pullForce   bool
	pullDryRun  bool
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull changes from Bowrain Server",
	Long: `Fetch changes from Bowrain Server and update local files.

Only changed blocks are transferred (incremental sync using content hashing).
Runs post-pull hooks if configured in .kapi/config.yaml.`,
	RunE: runPull,
}

func runPull(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return err
	}

	conn, err := project.NewSourceConnector(proj, formatReg)
	if err != nil {
		return err
	}
	defer conn.Close()

	locales := make([]model.LocaleID, len(pullLocales))
	for i, l := range pullLocales {
		locales[i] = model.LocaleID(l)
	}

	result, err := conn.Pull(cmd.Context(), connector.PullOptions{
		Locales: locales,
		Force:   pullForce,
		DryRun:  pullDryRun,
	})
	if err != nil {
		return err
	}

	if pullDryRun {
		fmt.Printf("Would pull %d blocks for %d locales\n", result.BlocksPulled, result.LocalesCount)
		return nil
	}

	if result.BlocksPulled == 0 {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("Pulled %d blocks for %d locales\n", result.BlocksPulled, result.LocalesCount)
	return nil
}

func init() {
	pullCmd.Flags().StringSliceVar(&pullLocales, "locale", nil, "Target locales to pull (e.g. fr-FR,de-DE)")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Overwrite local changes")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "Report what would be pulled without writing")
	rootCmd.AddCommand(pullCmd)
}
