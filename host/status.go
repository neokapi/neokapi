package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// StatusOutput is the structured result of `kapi status`: per-locale coverage
// and ship-gate standing for the project's tracked content.
type StatusOutput struct {
	Project string           `json:"project,omitempty"`
	Source  *SourceCoverage  `json:"source,omitempty"`
	Locales []LocaleCoverage `json:"locales"`
	// Server is the connected-server delta, contributed by the bowrain plugin's
	// server-status plumbing when the recipe has a server: block. Absent for a
	// pure local project. The cli module stays platform-neutral: it is populated
	// by shelling the plugin, never by importing bowrain.
	Server *StatusServerSection `json:"server,omitempty"`
	// Venue reports where `kapi up` would run the convergence loop. Present
	// only when the recipe declares a server: block — the venue of a plain
	// local project is not ambiguous, so it stays silent.
	Venue *StatusVenue `json:"venue,omitempty"`
}

// StatusVenue names the effective convergence venue for a server-connected
// recipe: where `kapi up` would run, the recipe's server.converge policy,
// and a note when the venue degrades (server declared but plumbing absent).
type StatusVenue struct {
	// Venue is "server" or "local".
	Venue string `json:"venue"`
	// ConvergePolicy echoes the recipe's server.converge value, when set.
	ConvergePolicy string `json:"converge_policy,omitempty"`
	// Note explains a degraded venue (e.g. the bowrain plugin is missing,
	// so a declared server cannot converge from this machine).
	Note string `json:"note,omitempty"`
}

// StatusServerSection is the connected-server standing merged into
// `kapi status`: ahead/behind transport counts plus any in-flight convergence
// runs. It mirrors the JSON the kapi-bowrain `server-status` command emits.
type StatusServerSection struct {
	ServerURL   string             `json:"server_url,omitempty"`
	Project     string             `json:"project,omitempty"`
	PendingPush int                `json:"pending_push"`
	PendingPull int                `json:"pending_pull"`
	LastSync    string             `json:"last_sync,omitempty"`
	ActiveRuns  []StatusActiveRun  `json:"active_runs,omitempty"`
	Terminology *StatusTerminology `json:"terminology,omitempty"`
}

// StatusTerminology is the local snapshot standing of the workspace's
// governed terminology: when the last concept pull ran and what it carried.
// nil when no concept pull has ever recorded a baseline — rendered as
// never-synced so stale local term checks are visible, not silent.
type StatusTerminology struct {
	PulledAt  string `json:"pulled_at"`
	Concepts  int    `json:"concepts"`
	Relations int    `json:"relations"`
}

// StatusActiveRun is one in-flight server convergence run in the status report.
type StatusActiveRun struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Passes int    `json:"passes"`
}

// statusLadder is the column order for the human grid.
var statusLadder = gate.TargetLadder()

// FormatText renders the coverage grid, implementing output.TextFormatter.
func (o StatusOutput) FormatText(w io.Writer) error {
	// Source readiness — how far the author's own content has progressed, shown
	// above the translation grid (the source feeds every target).
	if o.Source != nil && o.Source.Total > 0 {
		writeSourceLine(w, *o.Source)
	}
	if len(o.Locales) == 0 {
		// The server and venue standing still render below: a connected
		// project with nothing tracked yet must not hide where `up` would run.
		fmt.Fprintln(w, "No localized content tracked (no content collections with target locales).")
	} else {
		o.writeCoverageGrid(w)
	}
	if o.Server != nil {
		writeServerLine(w, *o.Server)
	}
	if o.Venue != nil {
		writeVenueLine(w, *o.Venue)
	}
	return nil
}

// writeCoverageGrid renders the per-locale coverage table. The scope column
// is the locale, or "locale/collection" when the project has named
// collections with their own gates.
func (o StatusOutput) writeCoverageGrid(w io.Writer) {
	headers := make([]string, 0, len(statusLadder)+3)
	headers = append(headers, "scope", "units")
	headers = append(headers, statusLadder...)
	headers = append(headers, "ship")

	t := output.NewTable(w).Accent(0).Headers(headers...)
	s := t.Styles()
	for _, lc := range o.Locales {
		cells := make([]string, 0, len(headers))
		cells = append(cells, scopeLabel(lc), strconv.Itoa(lc.Total))
		for _, rung := range statusLadder {
			cells = append(cells, fmt.Sprintf("%d%%", lc.Pct[rung]))
		}
		ship := shipCell(lc, s)
		// AI-approved units read as reviewed above, but honest provenance
		// matters: qualify how many of them an autonomous AI approved. Gates
		// only count these under `by: any` (core/gate approver classes).
		if lc.AIReviewed > 0 {
			ship += s.Muted.Render(fmt.Sprintf("  (%d reviewed by ai)", lc.AIReviewed))
		}
		t.Row(append(cells, ship)...)
	}
	t.Render()
}

// writeVenueLine renders the one-line convergence-venue standing for a
// server-connected recipe: where `kapi up` would run and under which
// server.converge policy.
func writeVenueLine(w io.Writer, v StatusVenue) {
	fmt.Fprintf(w, "venue   %s", v.Venue)
	if v.ConvergePolicy != "" {
		fmt.Fprintf(w, " · converge: %s", v.ConvergePolicy)
	}
	if v.Note != "" {
		fmt.Fprintf(w, " (%s)", v.Note)
	}
	fmt.Fprintln(w)
}

// writeServerLine renders the one-line connected-server standing beneath the
// coverage grid: the server, ahead/behind transport counts, and any in-flight
// convergence run.
func writeServerLine(w io.Writer, s StatusServerSection) {
	url := s.ServerURL
	if url == "" {
		url = "(connected)"
	}
	fmt.Fprintf(w, "\nserver  %s · %d to push · %d to pull", url, s.PendingPush, s.PendingPull)
	for _, r := range s.ActiveRuns {
		passes := ""
		if r.Passes > 0 {
			passes = fmt.Sprintf(" (pass %d)", r.Passes)
		}
		fmt.Fprintf(w, " · run %s %s%s", r.ID, r.State, passes)
	}
	fmt.Fprintln(w)
	if t := s.Terminology; t != nil {
		fmt.Fprintf(w, "terms   synced %s · %d concepts · %d relations\n", t.PulledAt, t.Concepts, t.Relations)
	} else {
		fmt.Fprintln(w, "terms   never synced — kapi pull snapshots the workspace terminology for offline checks")
	}
}

// sourceLadder is the column order for the source-readiness line.
var sourceLadder = gate.SourceLadder()

// writeSourceLine renders the one-line source-readiness summary: per-rung
// coverage of the author's content (labeled, since its ladder differs from the
// translation grid) plus its source-gate standing.
func writeSourceLine(w io.Writer, sc SourceCoverage) {
	cells := make([]string, 0, len(sourceLadder))
	for _, s := range sourceLadder {
		cells = append(cells, fmt.Sprintf("%s %d%%", s, sc.Pct[s]))
	}
	var standing string
	switch {
	case !sc.Gated:
		standing = ""
	case sc.Shippable:
		standing = "  ✓ ready"
	default:
		parts := make([]string, 0, len(sc.Pending))
		for _, sf := range sc.Pending {
			parts = append(parts, fmt.Sprintf("%s %d%%<%d%%", sf.State, int(sf.Actual), sf.Required))
		}
		standing = "  pending (" + strings.Join(parts, ", ") + ")"
	}
	fmt.Fprintf(w, "source: %d units  %s%s\n\n", sc.Total, strings.Join(cells, "  "), standing)
}

// scopeLabel renders a coverage row's scope: the locale, or "locale/collection"
// when the row is collection-scoped.
func scopeLabel(lc LocaleCoverage) string {
	if lc.Collection != "" {
		return lc.Locale + "/" + lc.Collection
	}
	return lc.Locale
}

// shipCell renders the ship column: shippable, pending (with the binding
// shortfall), or a dash when no gate applies to the locale.
func shipCell(lc LocaleCoverage, s *output.Styles) string {
	if !lc.Gated {
		return s.Dim("")
	}
	if lc.Shippable {
		return s.Success.Render("✓ shippable")
	}
	parts := make([]string, 0, len(lc.Pending))
	for _, sf := range lc.Pending {
		parts = append(parts, fmt.Sprintf("%s %d%%<%d%%", sf.State, int(sf.Actual), sf.Required))
	}
	return s.Warn.Render("pending (" + strings.Join(parts, ", ") + ")")
}

func (a *App) RunStatus(cmd Command, _ []string) error {
	a.InitRegistries()
	if cmd.Context() == nil {
		cmd.SetContext(context.Background())
	}

	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)

	sourceLang, _ := cmd.Flags().GetString("source-lang")
	if sourceLang == "" {
		sourceLang = string(proj.Defaults.SourceLanguage)
	}
	if sourceLang == "" {
		sourceLang = "en"
	}
	a.SourceLang = sourceLang

	localeFilter, _ := cmd.Flags().GetString("locale")
	units, err := a.resolveVerifyUnits(cmd, proj, root, nil, localeFilter)
	if err != nil {
		return fmt.Errorf("resolve content: %w", err)
	}

	return a.withParseCache(root, func() error {
		// --review lists the units awaiting human review (translated, not yet an
		// approved correction) — the review surface, the derived counterpart of the
		// convergence loop's "parked" outcome.
		if review, _ := cmd.Flags().GetBool("review"); review {
			items, qerr := a.computeReviewQueue(cmd.Context(), proj, root, units)
			if qerr != nil {
				return fmt.Errorf("compute review queue: %w", qerr)
			}
			return output.Print(cmd, reviewQueueOutput{Project: proj.Name, Pending: items})
		}

		cov, err := a.ComputeShipCoverage(cmd.Context(), proj, root, units, nil)
		if err != nil {
			return fmt.Errorf("compute coverage: %w", err)
		}

		// --ship emits the minimal picker manifest (locale → {shippable,
		// verified}) and stops — a build redirects it to ship.json, or writes it
		// with --emit. The richer coverage report is skipped: this shape is for a
		// language picker, not a dashboard.
		if ship, _ := cmd.Flags().GetBool("ship"); ship {
			return a.emitShipManifest(cmd, BuildShipManifest(cov))
		}

		src, err := a.computeSourceReadiness(cmd.Context(), proj, units)
		if err != nil {
			return fmt.Errorf("compute source readiness: %w", err)
		}

		out := StatusOutput{Project: proj.Name, Locales: cov}
		if src.Total > 0 {
			out.Source = &src
		}
		a.appendServerStatus(cmd, proj, &out)
		out.Venue = a.statusVenue(proj)
		a.WarnInertRecipeFields(cmd, proj)
		return output.Print(cmd, out)
	})
}

// appendServerStatus merges the connected-server delta into the status output
// when the recipe has a server: block and the bowrain plugin is installed. It
// shells the plugin's `server-status --json` (subprocess dispatch — the cli
// module never imports bowrain) and folds the result under out.Server. Any
// failure degrades to a one-line stderr warning and leaves the local report
// intact: a status command must never fail on a server hiccup.
func (a *App) appendServerStatus(cmd Command, proj *project.KapiProject, out *StatusOutput) {
	if _, ok := proj.Extras["server"]; !ok {
		return
	}
	if a.PluginHost == nil {
		return
	}
	route := a.PluginHost.CommandRoute("server-status")
	if route == nil {
		return
	}
	raw, err := route.CaptureStdout(cmd.Context(), "--json")
	if err != nil {
		if !a.Quiet {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read server status: %v\n", err)
		}
		return
	}
	var section StatusServerSection
	if err := json.Unmarshal(raw, &section); err != nil {
		if !a.Quiet {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not parse server status: %v\n", err)
		}
		return
	}
	out.Server = &section
}

// statusVenue derives the effective convergence venue for the status report.
// nil for a recipe with no server: block (no ambiguity to report). With a
// server: block, the venue is "server" when a plugin provides the server-up
// plumbing `kapi up` dispatches to, otherwise a degraded "local" with a note.
func (a *App) statusVenue(proj *project.KapiProject) *StatusVenue {
	spec, ok := serverRecipeSpec(proj)
	if !ok {
		return nil
	}
	v := &StatusVenue{ConvergePolicy: spec.Converge}
	if a.PluginHost != nil && a.PluginHost.CommandRoute("server-up") != nil {
		v.Venue = "server"
		return v
	}
	v.Venue = "local"
	v.Note = "bowrain plugin not installed — kapi up runs on this machine and does not push"
	return v
}

// AddStatusFlags registers the status command's flag surface on cmd —
// shared by the cobra factory and embedded surfaces (tests, review queue
// readers) so flag defaults stay identical everywhere.
func AddStatusFlags(cmd Command) {
	cmd.Flags().String("locale", "", "limit to a single target locale")
	cmd.Flags().String("source-lang", "", "source language (overrides the project's source_language)")
	cmd.Flags().Bool("review", false, "list translated units not yet approved in the project state store (the review worklist), instead of the coverage grid; approve a unit with `kapi apply` (kind:\"review\")")
	cmd.Flags().Bool("json", false, "output the structured result as JSON")
	cmd.Flags().Bool("ship", false, "emit the minimal ship.json picker manifest (locale → {shippable, verified}) instead of the coverage grid — the shape a language picker consumes to hide un-shippable locales and badge unverified ones AI")
	cmd.Flags().String("emit", "", "with --ship, write the manifest to this path (e.g. ship.json) instead of stdout")
}
