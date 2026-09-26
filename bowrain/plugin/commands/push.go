package commands

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/host/venue/transfer"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/neokapi/neokapi/host/venue/source"
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
	proj, err := requireProject(cmd)
	if err != nil {
		return err
	}
	conn, err := source.NewSourceConnector(app, proj, app.FormatReg)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = PushProject(cmd, proj, conn, transfer.PushOptions{
		Paths:  args,
		Force:  pushForce,
		DryRun: pushDryRun,
		Stream: pushStream,
	})
	return err
}

// PushedTerminologyError reports a push whose content landed and whose
// terminology fold failed. The push report has been printed by then.
type PushedTerminologyError struct{ Err error }

func (e *PushedTerminologyError) Error() string {
	return "the content above was pushed; its terminology was not: " + e.Err.Error()
}

func (e *PushedTerminologyError) Unwrap() error { return e.Err }

// PushAutomationError reports a recipe automation that failed around a push.
// Trigger is pre-push (nothing was pushed) or post-push (the push report has
// been printed).
type PushAutomationError struct {
	Trigger string
	Err     error
}

func (e *PushAutomationError) Error() string { return e.Trigger + " automation: " + e.Err.Error() }

func (e *PushAutomationError) Unwrap() error { return e.Err }

// PushProject is the one push of a project, whichever route it arrives by:
// `kapi-bowrain push`, and the daemon that serves `kapi push`. It runs the
// recipe's pre-push automations, pushes the content with the recipe's declared
// context, folds the workspace terminology in, prints the push report to cmd,
// then runs the post-push automations. Automations and the flows they run
// print to cmd as well.
//
// The returned report is nil only when nothing was pushed. A terminology
// failure returns the report with a *PushedTerminologyError, and a failed
// automation returns a *PushAutomationError. The caller owns conn.
func PushProject(cmd *cobra.Command, proj *project.Project, conn *source.BowrainSourceConnector, opts transfer.PushOptions) (*PushReport, error) {
	if err := runLocalAutomations(cmd, proj, project.HookPrePush); err != nil {
		return nil, &PushAutomationError{Trigger: project.HookPrePush, Err: err}
	}

	pr, err := transfer.PushProject(cmd.Context(), app, proj, conn, opts)
	if err != nil {
		return nil, err
	}

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
	// A terminology failure is returned after the report prints, so the
	// content that landed and its push id stay in front of the user.
	cres, conceptErr := conceptPush(cmd.Context(), proj, opts.DryRun)
	if cres != nil {
		out.ConceptsApplied = cres.ConceptsApplied
		out.RelationsApplied = cres.RelationsApplied
		out.ConceptsProposed = cres.ConceptsProposed
		out.ChangesetID = cres.ChangesetID
		out.ChangesetURL = cres.ChangesetURL
		out.ChangesetUnchanged = cres.ChangesetUnchanged
	}
	applyLoopStatus(&out, proj, conn.Stream())

	report := &PushReport{PushOutput: out, PushID: pr.PushID, ChunkCount: pr.ChunkCount}
	if err := output.Print(cmd, out); err != nil {
		return report, err
	}
	if conceptErr != nil {
		return report, &PushedTerminologyError{Err: conceptErr}
	}

	if err := runLocalAutomations(cmd, proj, project.HookPostPush); err != nil {
		return report, &PushAutomationError{Trigger: project.HookPostPush, Err: err}
	}
	return report, nil
}

// PushReport is what PushProject pushed: the report it printed, and the
// transport details a daemon response carries beside it.
type PushReport struct {
	output.PushOutput
	PushID     string
	ChunkCount int
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
	addProjectFlag(pushCmd)
	pushCmd.Flags().BoolVar(&pushForce, "force", false, "Re-upload everything, even unchanged blocks")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "Show what would be uploaded without sending")
	pushCmd.Flags().StringVar(&pushStream, "stream", "", "Target stream (default: auto-detect from git/CI)")
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(pushCmd) })
}
