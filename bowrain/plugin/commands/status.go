package commands

import (
	"encoding/json"

	"github.com/neokapi/neokapi/bowrain/core/project"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

// serverStatusCmd is the plumbing behind the built-in `kapi status` server
// section: it emits the project's sync standing and in-flight convergence runs
// as JSON on stdout. The built-in status command shells out to this
// (`kapi-bowrain command server-status --json`) when the recipe has a server:
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
}

// terminologyJSON is the local snapshot standing of the workspace's governed
// terminology — the concept baseline a pull records into the sync cache.
// Absent when no concept pull has ever run; the built-in status renders that
// as never-synced.
type terminologyJSON struct {
	PulledAt  string `json:"pulled_at"`
	Concepts  int    `json:"concepts"`
	Relations int    `json:"relations"`
}

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

	// Terminology snapshot standing, read straight from the sync cache so it
	// costs no server round-trip and reports even when the server is down.
	if b := project.LoadSyncCache(proj.Layout).ConceptBaseline; b != nil {
		out.Terminology = &terminologyJSON{
			PulledAt:  b.PulledAt.Format(timeRFC3339),
			Concepts:  len(b.Concepts),
			Relations: len(b.Relations),
		}
	}

	conn, err := bconn.NewSourceConnector(proj, app.FormatReg)
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

func init() {
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(serverStatusCmd) })
}
