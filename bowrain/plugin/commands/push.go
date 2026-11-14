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

Only changed blocks are sent. Runs the recipe's pre-push automations if any.

One push is one transaction on the server. Beside the content it carries:

  - the declared structure: the collections the recipe names, the point each
    occupies in the project's context space, and the voice governing it;
  - terminology edits, when the project is claimed into a workspace. Ordinary
    edits (definitions, notes, proposed terms, non-governed relations) apply
    directly. Governed edits (a term set to forbidden or preferred, a
    REPLACED_BY relation, a concept delete) become one change-set for review;
  - the context ledger, when the recipe keeps the project's context on the
    server (context: {backend: bowrain}): the recorded operations the server
    does not hold yet.

The server applies all of it or none of it. A refused terminology edit or
ledger operation leaves the content where it was, and the push report lists
each part.

Collections are created on first push, unchanged content is a no-op, and a
changed voice lands as a new version with server-side edits archived rather
than overwritten. A collection the recipe no longer names is reported, never
deleted.`,
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
// context, the project's terminology edits and (when the context lives on the
// server) its context ledger in one transaction, prints the push report to
// cmd, then runs the post-push automations. Automations and the flows they run
// print to cmd as well.
//
// The returned report is nil only when nothing was pushed. A failed
// automation returns a *PushAutomationError. The caller owns conn.
func PushProject(cmd *cobra.Command, proj *project.Project, conn *source.BowrainSourceConnector, opts transfer.PushOptions) (*PushReport, error) {
	if err := runLocalAutomations(cmd, proj, project.HookPrePush); err != nil {
		return nil, &PushAutomationError{Trigger: project.HookPrePush, Err: err}
	}

	// The workspace terminology travels in the push: local concept and
	// relation edits are classified against the pulled baseline here, and the
	// server applies them in the push's transaction. Skipped silently only
	// when the project is not claimed into a workspace.
	terms, err := conceptEdits(cmd.Context(), proj)
	if err != nil {
		return nil, fmt.Errorf("terminology: %w", err)
	}
	opts.Terms = terms

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
		SettingsApplied:       pr.SettingsApplied,
		SettingsRefused:       pr.SettingsRefused,
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

	if cres := conceptResult(proj, pr.Terms, pr.DryRun, pr.PartsPending); cres != nil {
		out.ConceptsApplied = cres.ConceptsApplied
		out.RelationsApplied = cres.RelationsApplied
		out.ConceptsProposed = cres.ConceptsProposed
		out.ChangesetID = cres.ChangesetID
		out.ChangesetURL = cres.ChangesetURL
		out.ChangesetUnchanged = cres.ChangesetUnchanged
	}
	if pr.Ledger != nil {
		out.ContextObjects = pr.Ledger.Objects
		out.ContextOperations = pr.Ledger.Operations
		out.ContextBound = pr.Ledger.Bound
	}
	out.PartsPending = pr.PartsPending && !pr.DryRun
	out.CollectionsDeclared = pr.CollectionsDeclared
	applyLoopStatus(&out, proj, conn.Stream())

	report := &PushReport{PushOutput: out, PushID: pr.PushID, ChunkCount: pr.ChunkCount}
	if err := output.Print(cmd, out); err != nil {
		return report, err
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
