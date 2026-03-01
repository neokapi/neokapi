package main

import (
	"github.com/gokapi/gokapi/brain/cmd/brain/output"
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

func runPull(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return err
	}

	conn, err := project.NewSourceConnector(proj, app.FormatReg)
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

	out := output.PullOutput{
		BlocksPulled: result.BlocksPulled,
		LocalesCount: result.LocalesCount,
		FilesWritten: result.FilesWritten,
	}
	if pullDryRun {
		out.DryRun = true
	} else if result.BlocksPulled == 0 {
		out.UpToDate = true
	}

	return output.Print(cmd, out)
}

func init() {
	pullCmd.Flags().StringSliceVar(&pullLocales, "locale", nil, "languages to download (e.g. fr,de)")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Re-download everything, even unchanged content")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "Show what would change without writing files")
	rootCmd.AddCommand(pullCmd)
}
