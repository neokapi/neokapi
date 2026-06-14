package commands

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

var (
	pullLocales []string
	pullForce   bool
	pullDryRun  bool
	pullStream  string
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Download translations and governed terminology from the server",
	Long: `Download translations from the server and update local files.

Only changed blocks are transferred. Runs post-pull hooks if configured.

When the project is claimed into a workspace, pull also snapshots the
workspace's governed concepts and their relations into the project's bound
termbase (.kapi/termbase.db) and records a baseline, so a later 'kapi push'
can diff local terminology edits against it and 'kapi verify --terms' gates
offline against the same governed vocabulary.`,
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
func doPull(ctx context.Context, conn *bconn.BowrainSourceConnector, locales []string, force, dryRun bool) (*PullResult, error) {
	if conn == nil {
		proj, err := project.FindProject("")
		if err != nil {
			return nil, err
		}
		var connErr error
		conn, connErr = bconn.NewSourceConnector(proj, app.FormatReg)
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
	// Run pre-pull automations.
	if proj := findProjectForAutomations(); proj != nil {
		if err := runLocalAutomations(cmd, proj, "pre-pull"); err != nil {
			return fmt.Errorf("pre-pull automation: %w", err)
		}
	}

	// Create connector and apply --stream override.
	proj, err := project.FindProject("")
	if err != nil {
		return err
	}
	conn, err := bconn.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		return err
	}
	defer conn.Close()

	if pullStream != "" {
		conn.SetStream(pullStream)
	}

	result, err := doPull(cmd.Context(), conn, pullLocales, pullForce, pullDryRun)
	if err != nil {
		return err
	}

	out := output.PullOutput{
		BlocksPulled: result.BlocksPulled,
		LocalesCount: result.LocalesCount,
		FilesWritten: result.FilesWritten,
		Stream:       conn.Stream(),
		DryRun:       result.DryRun,
		UpToDate:     result.UpToDate,
	}

	// Fold the workspace's governed terminology into the pull: fetch the
	// concepts + relations into the project's bound termbase (skipped silently
	// when the project is not workspace-claimed). The baseline is recorded on the
	// connector's in-memory cache so the single deferred conn.Close() below
	// persists it together with the block-sync state — writing it to disk here
	// would be overwritten by that Close().
	cres, baseline, cerr := conceptPull(cmd.Context(), proj, pullDryRun)
	if cerr != nil {
		return cerr
	}
	if baseline != nil {
		conn.SetConceptBaseline(baseline)
	}
	if cres != nil {
		out.ConceptsPulled = cres.Concepts
		out.ConceptRelationsPulled = cres.Relations
	}

	if err := output.Print(cmd, out); err != nil {
		return err
	}

	// Run post-pull automations.
	if proj := findProjectForAutomations(); proj != nil {
		if err := runLocalAutomations(cmd, proj, "post-pull"); err != nil {
			return fmt.Errorf("post-pull automation: %w", err)
		}
	}

	return nil
}

func init() {
	pullCmd.Flags().StringSliceVar(&pullLocales, "locale", nil, "languages to download (e.g. fr,de)")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Re-download everything, even unchanged content")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "Show what would change without writing files")
	pullCmd.Flags().StringVar(&pullStream, "stream", "", "Source stream (default: auto-detect from git/CI)")
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(pullCmd) })
}
