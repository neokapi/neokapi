package main

import (
	"context"

	"github.com/gokapi/gokapi/bowrain-cli/cmd/bowrain/output"
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
	Short: "Download translations from the server",
	Long: `Download translations from the server and update local files.

Only changed blocks are transferred. Runs post-pull hooks if configured.`,
	RunE: runPull,
}

// PullResult holds the structured result of a pull operation.
type PullResult struct {
	BlocksPulled int
	LocalesCount int
	FilesWritten int
	DryRun       bool
	UpToDate     bool
}

// doPull executes the core pull logic and returns structured results.
// If conn is provided, it is used; otherwise a new connector is created.
func doPull(ctx context.Context, conn *project.BrainSourceConnector, locales []string, force, dryRun bool) (*PullResult, error) {
	if conn == nil {
		proj, err := project.FindProject("")
		if err != nil {
			return nil, err
		}
		var connErr error
		conn, connErr = project.NewSourceConnector(proj, app.FormatReg)
		if connErr != nil {
			return nil, connErr
		}
		defer conn.Close()
	}

	modelLocales := make([]model.LocaleID, len(locales))
	for i, l := range locales {
		modelLocales[i] = model.LocaleID(l)
	}

	result, err := conn.Pull(ctx, connector.PullOptions{
		Locales: modelLocales,
		Force:   force,
		DryRun:  dryRun,
	})
	if err != nil {
		return nil, err
	}

	pr := &PullResult{
		BlocksPulled: result.BlocksPulled,
		LocalesCount: result.LocalesCount,
		FilesWritten: result.FilesWritten,
	}
	if dryRun {
		pr.DryRun = true
	} else if result.BlocksPulled == 0 {
		pr.UpToDate = true
	}

	return pr, nil
}

func runPull(cmd *cobra.Command, args []string) error {
	result, err := doPull(cmd.Context(), nil, pullLocales, pullForce, pullDryRun)
	if err != nil {
		return err
	}

	out := output.PullOutput{
		BlocksPulled: result.BlocksPulled,
		LocalesCount: result.LocalesCount,
		FilesWritten: result.FilesWritten,
		DryRun:       result.DryRun,
		UpToDate:     result.UpToDate,
	}

	return output.Print(cmd, out)
}

func init() {
	pullCmd.Flags().StringSliceVar(&pullLocales, "locale", nil, "languages to download (e.g. fr,de)")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Re-download everything, even unchanged content")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "Show what would change without writing files")
	rootCmd.AddCommand(pullCmd)
}
