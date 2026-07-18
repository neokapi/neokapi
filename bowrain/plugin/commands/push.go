package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

var (
	pushForce        bool
	pushDryRun       bool
	pushStream       string
	pushConceptsOnly bool
)

var pushCmd = &cobra.Command{
	Use:   "push [paths...]",
	Short: "Upload local changes and terminology edits to the server",
	Long: `Upload local changes to the server.

Only changed blocks are sent. Runs pre-push hooks if configured.

When the project is claimed into a workspace and a baseline was pulled, push
also reconciles local terminology edits against that baseline. Ordinary edits
(definitions, notes, proposed terms, non-governed relations) apply directly,
while governed edits (a term set to forbidden/preferred, a REPLACED_BY
relation, a concept delete) are bundled into a single change-set proposal for
review — the same separation of duties the web hub enforces. Push reports what
applied directly versus what was proposed.`,
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
func doPush(ctx context.Context, opts connector.PushOptions, args []string) (*PushResult, *bconn.BowrainSourceConnector, error) {
	proj, err := project.FindProject("")
	if err != nil {
		return nil, nil, err
	}

	conn, err := bconn.NewSourceConnector(proj, app.FormatReg)
	if err != nil {
		return nil, nil, err
	}

	// Apply --stream flag override (takes priority over config/auto-detect).
	if pushStream != "" {
		conn.SetStream(pushStream)
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
	// --concepts: terminology-only transport. Reconcile local termbase edits
	// against the pulled baseline (direct edits + governed change-set) without
	// moving any content blocks and without firing push hooks — the mirror of
	// `kapi pull --concepts`.
	if pushConceptsOnly {
		proj, err := project.FindProject("")
		if err != nil {
			return err
		}
		out := output.PushOutput{DryRun: pushDryRun}
		if cres, cerr := conceptPush(cmd.Context(), proj, pushDryRun); cerr != nil {
			return cerr
		} else if cres != nil {
			out.ConceptsApplied = cres.ConceptsApplied
			out.RelationsApplied = cres.RelationsApplied
			out.ConceptsProposed = cres.ConceptsProposed
			out.ChangesetID = cres.ChangesetID
			out.ChangesetURL = cres.ChangesetURL
		}
		return output.Print(cmd, out)
	}

	// Run pre-push automations.
	if proj := findProjectForAutomations(); proj != nil {
		if err := runLocalAutomations(cmd, proj, "pre-push"); err != nil {
			return fmt.Errorf("pre-push automation: %w", err)
		}
	}

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
		Stream:       conn.Stream(),
		DryRun:       pr.DryRun,
		UpToDate:     pr.UpToDate,
	}

	// Fold the workspace's governed terminology into the push: reconcile local
	// concept/relation edits against the pulled baseline (ordinary edits go up
	// directly, governed edits become a submitted change-set). Skipped silently
	// when the project is not workspace-claimed or has no pulled baseline.
	if proj, perr := project.FindProject(""); perr == nil {
		if cres, cerr := conceptPush(cmd.Context(), proj, pushDryRun); cerr != nil {
			return cerr
		} else if cres != nil {
			out.ConceptsApplied = cres.ConceptsApplied
			out.RelationsApplied = cres.RelationsApplied
			out.ConceptsProposed = cres.ConceptsProposed
			out.ChangesetID = cres.ChangesetID
			out.ChangesetURL = cres.ChangesetURL
		}
		applyLoopStatus(&out, proj, conn.Stream())
	}

	if err := output.Print(cmd, out); err != nil {
		return err
	}

	// Run post-push automations.
	if proj := findProjectForAutomations(); proj != nil {
		if err := runLocalAutomations(cmd, proj, "post-push"); err != nil {
			return fmt.Errorf("post-push automation: %w", err)
		}
	}

	return nil
}

// applyLoopStatus fills the push output's loop footer from the recipe's server
// block: the effective convergence policy (server.converge; on-push when unset)
// and the project's web destinations. URLs derive from the compound project URL
// the way changesetURL does; they need a workspace slug, since the web surfaces
// live under /<workspace>/. Review work lands on the workspace tasks queue.
func applyLoopStatus(out *output.PushOutput, proj *project.Project, stream string) {
	if proj.Recipe == nil || proj.Recipe.Server == nil || proj.Recipe.Server.URL == "" {
		return
	}
	server := proj.Recipe.Server
	out.Converge = string(server.ResolvedConverge())

	base := strings.TrimRight(server.ServerURL(), "/")
	ws := server.Workspace()
	pid := server.ProjectID()
	if base == "" || ws == "" || pid == "" {
		return
	}
	if stream == "" {
		stream = "main"
	}
	out.ProjectURL = fmt.Sprintf("%s/%s/p/%s/s/%s", base, ws, pid, stream)
	out.ReviewURL = fmt.Sprintf("%s/%s/tasks", base, ws)
}

func init() {
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Re-upload everything, even unchanged blocks")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "Show what would be uploaded without sending")
	pushCmd.Flags().StringVar(&pushStream, "stream", "", "Target stream (default: auto-detect from git/CI)")
	pushCmd.Flags().BoolVar(&pushConceptsOnly, "concepts", false, "Sync only local terminology edits to the workspace (direct edits + governed change-set); no content transport, no hooks")
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(pushCmd) })
}
