package commands

import (
	"context"
	"encoding/json"

	"github.com/neokapi/neokapi/bowrain/core/config"
	"github.com/neokapi/neokapi/bowrain/core/project"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/spf13/cobra"
)

// serverStatusCmd is the plumbing behind the built-in `kapi status` server
// section: it emits the project's sync standing and in-flight convergence runs
// as JSON on stdout. The built-in status command shells out to this
// (`kapi-bowrain command server-status --json`) when the recipe has a bowrain:
// block and merges the result under a "server" section — one standing report
// across the transport and convergence layers.
//
// It replaces the old top-level plugin `status` command; project standing is
// now owned by the built-in `kapi status` (derived coverage + this server
// delta), not a second differently-shaped status.
var serverStatusCmd = &cobra.Command{
	Use:    "server-status",
	Short:  "Emit the server sync + loop-run standing as JSON (plumbing for kapi status)",
	Hidden: true,
	RunE:   runServerStatus,
}

// serverStatusJSON is the exact shape the built-in `kapi status` merges under
// its "server" section.
type serverStatusJSON struct {
	ServerURL   string           `json:"server_url"`
	Project     string           `json:"project"`
	PendingPush int              `json:"pending_push"`
	PendingPull int              `json:"pending_pull"`
	LastSync    string           `json:"last_sync,omitempty"`
	ActiveRuns  []activeRunJSON  `json:"active_runs"`
	Terminology *terminologyJSON `json:"terminology,omitempty"`
	Governance  *governanceJSON  `json:"governance,omitempty"`
}

// governanceJSON is the governance axis of the built-in status report: the ref
// this project last observed, and the one the venue publishes now.
//
// Both refs travel; no verdict does. Comparison is core/ref's, made once where
// the report is rendered — a second comparison here would be a second answer to
// "did governance move?", and the two would part company the day one of them
// learned a component the other had not.
type governanceJSON struct {
	Observed   ref.Ref  `json:"observed"`
	Venue      *ref.Ref `json:"venue,omitempty"`
	ObservedAt string   `json:"observed_at,omitempty"`
}

// terminologyJSON is the standing of the workspace's governed terminology: the
// local snapshot a pull records into the sync cache, plus what this project has
// proposed and not yet had reviewed.
//
// The snapshot half is absent when no concept pull has ever run, and the
// built-in status renders that as never-synced. That was the whole of the
// terms line, and it read wrong in the one case that matters most: a push had
// just submitted a change-set of governed edits, the pull that would refresh
// the baseline cannot run until it is reviewed, and status therefore reported
// "never synced" about a project whose terminology was in flight. Awaiting
// review is not the same as never sent.
type terminologyJSON struct {
	// PulledAt is when this project last reached the server, from the
	// destination-keyed freshness refs. It reports; it decides nothing. What
	// "still current?" is answered by is the ref's terms component, which is an
	// identity of the terminology rather than a clock beside it.
	PulledAt  string `json:"pulled_at"`
	Concepts  int    `json:"concepts"`
	Relations int    `json:"relations"`

	// PendingChangesets is how many change-sets this workspace holds awaiting
	// review, with the newest one's id and a link. Read from the server, so it
	// is omitted (not zero) when the server could not be asked.
	PendingChangesets int    `json:"pending_changesets,omitempty"`
	PendingChangeset  string `json:"pending_changeset_id,omitempty"`
	PendingURL        string `json:"pending_changeset_url,omitempty"`
}

// changesetInReview is the server's status for a submitted change-set awaiting
// review (knowledge.ChangeSetInReview). A persisted status value, so it is
// spelled here exactly as the server stores it.
const changesetInReview = "in_review"

type activeRunJSON struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Passes int    `json:"passes"`
}

func runServerStatus(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	out := serverStatusJSON{ActiveRuns: []activeRunJSON{}}

	proj, err := project.FindProject("")
	if err != nil {
		return err
	}
	if proj.Recipe.Server != nil {
		out.ServerURL = proj.Recipe.Server.ServerURL()
		out.Project = proj.Recipe.Server.ProjectID()
	}

	// Terminology snapshot standing, read straight from the local caches so it
	// costs no server round-trip and reports even when the server is down.
	if b := project.LoadSyncCache(proj.Layout).ConceptBaseline; b != nil {
		out.Terminology = &terminologyJSON{
			Concepts:  len(b.Concepts),
			Relations: len(b.Relations),
		}
		if proj.Recipe.Server != nil {
			refs := refcache.Load(proj.Layout,
				config.NormalizeServerURL(proj.Recipe.Server.ServerURL()),
				proj.Recipe.Server.ProjectID())
			if !refs.ObservedAt.IsZero() {
				out.Terminology.PulledAt = refs.ObservedAt.Format(timeRFC3339)
			}
		}
	}
	// And what this workspace has proposed and not had reviewed. This half does
	// cost a round-trip, and is best-effort for that reason: a server that
	// cannot be reached leaves the local snapshot standing exactly as before.
	addPendingChangesets(ctx, proj, &out)

	conn, err := bconn.NewSourceConnector(app, proj, app.FormatReg)
	if err != nil {
		// No server configured — return the empty standing rather than error,
		// so the built-in status degrades gracefully.
		return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
	}
	defer conn.Close()

	if status, serr := conn.Status(ctx); serr == nil {
		out.PendingPush = status.PendingPush
		out.PendingPull = status.PendingPull
		if !status.LastSync.IsZero() {
			out.LastSync = status.LastSync.Format(timeRFC3339)
		}
	}

	// The governance axis. It costs one round trip, which is why it is taken
	// here and not on every read path: a status report is a question asked on
	// purpose, and the retrieval surfaces answer from what this one records.
	observed, venue, observedAt := conn.Governance(ctx)
	out.Governance = &governanceJSON{Observed: observed, Venue: venue}
	if !observedAt.IsZero() {
		out.Governance.ObservedAt = observedAt.Format(timeRFC3339)
	}

	// List in-flight runs (best-effort; a server that predates convergence runs
	// simply reports none).
	if client := conn.Client(); client != nil {
		if runs, rerr := client.ListConvergenceRuns(ctx, 10); rerr == nil {
			for _, r := range runs {
				if r.State == "running" {
					out.ActiveRuns = append(out.ActiveRuns, activeRunJSON{ID: r.ID, State: r.State, Passes: r.Passes})
				}
			}
		}
	}

	return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

// addPendingChangesets fills in the terminology awaiting review.
//
// It is what makes the terms line able to say "proposed, awaiting review"
// rather than "never synced". The two are easy to confuse from the client's
// side, because they look identical in the sync cache: a governed edit becomes
// a change-set, the change-set holds it until a reviewer merges it, and until
// then no pull can bring it back down as a baseline. The distinction only
// exists on the server, so the client has to ask.
//
// Best-effort throughout: an unreachable server, an unclaimed project, or a
// server too old for the endpoint leaves the local snapshot standing as it was.
// Status must report what it can from a laptop on a train.
func addPendingChangesets(ctx context.Context, proj *project.Project, out *serverStatusJSON) {
	client, err := knowledgeClient(proj)
	if err != nil || client == nil {
		return
	}
	sets, err := client.ListChangesets(ctx, changesetInReview)
	if err != nil || len(sets) == 0 {
		return
	}
	if out.Terminology == nil {
		out.Terminology = &terminologyJSON{}
	}
	out.Terminology.PendingChangesets = len(sets)
	// Newest first, per the endpoint's contract — the head is the one a user
	// who just pushed is looking for.
	out.Terminology.PendingChangeset = sets[0].ID
	out.Terminology.PendingURL = changesetURL(proj, sets[0].ID)
}

func init() {
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(serverStatusCmd) })
}
