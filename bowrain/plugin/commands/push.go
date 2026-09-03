package commands

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/host/venue/transfer"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/spf13/cobra"
)

var (
	pushForce  bool
	pushDryRun bool
	pushStream string
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
review, the same separation of duties the web hub enforces. Push reports what
applied directly versus what was proposed.

Push also carries the project's declared context: the collections the recipe
names, the point each occupies in the project's context space, and the brand
voice governing it. They travel inside the push, so the collections a pushed
item belongs to exist server-side by the time the item is stored. They are created on
first push, unchanged content is a no-op, and a changed voice lands as a new
version with server-side edits archived rather than overwritten. A collection
the recipe no longer names is reported, never deleted. Use the recipe to carry
the structure without the governance.`,
	RunE: runPush,
}

func runPush(cmd *cobra.Command, args []string) error {
	// Run pre-push automations.
	if proj := findProjectForAutomations(); proj != nil {
		if err := runLocalAutomations(cmd, proj, "pre-push"); err != nil {
			return fmt.Errorf("pre-push automation: %w", err)
		}
	}

	pr, conn, err := transfer.Push(cmd.Context(), app, transfer.PushOptions{
		Paths:  args,
		Force:  pushForce,
		DryRun: pushDryRun,
		Stream: pushStream,
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	out := output.PushOutput{
		BlocksPushed:          pr.BlocksPushed,
		BlocksUploaded:        pr.BlocksUploaded,
		WordCount:             pr.WordCount,
		FilesScanned:          pr.FilesScanned,
		Stream:                conn.Stream(),
		DryRun:                pr.DryRun,
		UpToDate:              pr.UpToDate,
		UndeclaredCollections: pr.UndeclaredCollections,
		AssetsPushed:          pr.AssetsPushed,
		AssetsFailed:          pr.AssetsFailed,
		AssetErrors:           pr.AssetErrors,
		Ingest:                pr.Ingest,
		VerdictsRetired:       pr.VerdictsRetired,
	}
	if pr.Governance != nil {
		out.VerdictsRefused = pr.Governance.Refusals
	}
	if pr.Brand != nil {
		out.VoiceProfile = pr.Brand.Name
		out.VoiceAction = pr.Brand.Action
		out.VoiceVersion = pr.Brand.Version
		out.VoiceReason = pr.Brand.Reason
	}

	// Fold the workspace's governed terminology into the push: reconcile local
	// concept/relation edits against the pulled baseline (ordinary edits go up
	// directly, governed edits become a submitted change-set). Skipped silently
	// only when the project is not claimed into a workspace.
	//
	// A terminology failure is carried past the report rather than returned
	// through it. Returning here threw away the content result: the blocks HAD
	// been uploaded and stored, and the user was shown only the terminology
	// error — reading, reasonably, that the whole push failed, and losing the
	// push id that identifies what did land.
	var conceptErr error
	if proj, perr := project.FindProject(""); perr == nil {
		cres, cerr := conceptPush(cmd.Context(), proj, pushDryRun)
		conceptErr = cerr
		if cres != nil {
			out.ConceptsApplied = cres.ConceptsApplied
			out.RelationsApplied = cres.RelationsApplied
			out.ConceptsProposed = cres.ConceptsProposed
			out.ChangesetID = cres.ChangesetID
			out.ChangesetURL = cres.ChangesetURL
			out.ChangesetUnchanged = cres.ChangesetUnchanged
		}
		applyLoopStatus(&out, proj, conn.Stream())
	}

	if err := output.Print(cmd, out); err != nil {
		return err
	}
	if conceptErr != nil {
		return fmt.Errorf("the content above was pushed; its terminology was not: %w", conceptErr)
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
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(pushCmd) })
}
