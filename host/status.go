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
	"github.com/neokapi/neokapi/core/model"
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
	// Staged counts decisions recorded but not yet written to the project's
	// committed record. Omitted when there are none, so a clean project stays
	// quiet — the same habit as git status.
	Staged int `json:"staged,omitempty"`
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

// StatusTerminology is the standing of the workspace's governed terminology:
// when the last concept pull ran and what it carried, plus what this project
// has proposed and not yet had reviewed. nil when neither is known — rendered
// as never-synced so stale local term checks are visible, not silent.
type StatusTerminology struct {
	PulledAt  string `json:"pulled_at"`
	Concepts  int    `json:"concepts"`
	Relations int    `json:"relations"`

	// PendingChangesets is how many governed terminology change-sets are
	// awaiting review, with the newest one's id and review link. A pushed
	// governed edit lives here until a reviewer merges it, and no pull can
	// bring it down as a baseline in the meantime — so without this the terms
	// line reported "never synced" about terminology that had in fact been
	// sent, received, and was sitting in someone's queue.
	PendingChangesets int    `json:"pending_changesets,omitempty"`
	PendingChangeset  string `json:"pending_changeset_id,omitempty"`
	PendingURL        string `json:"pending_changeset_url,omitempty"`
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
	writeStagedLine(w, o.Staged)
	return nil
}

// writeStagedLine reports unit state recorded but not yet committed, and says
// what to run. Silent when there is none: a clean project should read clean,
// the same habit as git status.
func writeStagedLine(w io.Writer, staged int) {
	if staged <= 0 {
		return
	}
	noun := "changes"
	if staged == 1 {
		noun = "change"
	}
	fmt.Fprintf(w, "\n%d unit-state %s staged, not committed — run `kapi commit` to write them to the project record.\n",
		staged, noun)
}

// writeCoverageGrid renders the per-locale coverage table. The scope column
// is the locale, or "locale/collection" when the project has named
// collections with their own gates.
//
// Ship is a verdict, not a percentage. A percentage there would invite the
// question "is 80% shippable?", whose answer is always no — a gate either holds
// or it does not. So the column reads `ready` or `blocked: <rung>`, naming the
// first unmet gate so it points at the work; the numbers live in the stage
// columns, and the pipeline bar carries distance-to-the-bar at a glance.
func (o StatusOutput) writeCoverageGrid(w io.Writer) {
	headers := make([]string, 0, len(statusLadder)+4)
	headers = append(headers, "scope", "units")
	headers = append(headers, statusLadder...)
	headers = append(headers, "pipeline", "ship")

	t := output.NewTable(w).Accent(0).Headers(headers...)
	s := t.Styles()
	bars := shipBars(w)
	for _, lc := range o.Locales {
		cells := make([]string, 0, len(headers))
		cells = append(cells, scopeLabel(lc), strconv.Itoa(lc.Total))
		for _, rung := range statusLadder {
			cells = append(cells, fmt.Sprintf("%d%%", lc.Pct[rung]))
		}
		cells = append(cells, pipelineCell(lc, bars))
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
	o.writeShipSummary(w, s)
}

// writeShipSummary closes the grid with the one number a release decision needs:
// how many gated scopes clear their bar. Omitted when nothing is gated, since
// "0 of 0 ready" says nothing.
func (o StatusOutput) writeShipSummary(w io.Writer, s *output.Styles) {
	gated, ready := 0, 0
	for _, lc := range o.Locales {
		if !lc.Gated {
			continue
		}
		gated++
		if lc.Shippable {
			ready++
		}
	}
	if gated == 0 {
		return
	}
	line := fmt.Sprintf("%d of %d scopes ready to ship", ready, gated)
	if ready == gated {
		fmt.Fprintf(w, "\n%s\n", s.Success.Render(line))
		return
	}
	fmt.Fprintf(w, "\n%s\n", line)
}

// shipBarWidth is the pipeline bar's width on a normal terminal.
const shipBarWidth = 10

// shipGridMinWidth is the terminal width below which the pipeline column drops
// its bar and shows the percentage alone.
const shipGridMinWidth = 100

// shipBars decides whether the pipeline column draws a block bar. A narrow
// terminal gets the percentage alone rather than a bar squeezed to noise; the
// number is the information, the bar is the affordance.
func shipBars(w io.Writer) bool {
	if !writerIsTerminal(w) {
		// A pipe or a captured buffer keeps the bar: it is plain UTF-8, not an
		// escape sequence, so a log or a golden file reads fine. Only width is
		// a reason to drop it, and a non-terminal has no width to respect.
		return true
	}
	return output.TerminalWidth(w) >= shipGridMinWidth
}

// pipelineCell renders the distance-to-ship column: a block bar plus the
// percentage, or the percentage alone when there is no room. An ungated scope
// has no bar to fill, so it reads as a dash — the same "not applicable" the
// ship column shows.
func pipelineCell(lc LocaleCoverage, bars bool) string {
	if !lc.Gated {
		return "—"
	}
	if !bars {
		return fmt.Sprintf("%3d%%", lc.ShipProgress)
	}
	return fmt.Sprintf("%s %3d%%", ProgressBar(lc.ShipProgress, 100, shipBarWidth), lc.ShipProgress)
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
	writeTermsLine(w, s.Terminology)
}

// writeTermsLine renders the terminology standing beneath the server line.
//
// Three states, not two. "Synced" and "never synced" were the whole vocabulary,
// and between them sat the state a user is most likely to be in right after a
// push: governed edits proposed and awaiting review. Those cannot appear in the
// local baseline — a change-set holds them until a reviewer merges it — so a
// project that had just sent its terminology upstream was told it had never
// sent any, which is the report that sends someone to re-do work already
// queued in front of a colleague.
func writeTermsLine(w io.Writer, t *StatusTerminology) {
	switch {
	case t == nil:
		fmt.Fprintln(w, "terms   never synced — kapi pull snapshots the workspace terminology for offline checks")
		return
	case t.PulledAt != "":
		fmt.Fprintf(w, "terms   synced %s · %d concepts · %d relations", t.PulledAt, t.Concepts, t.Relations)
	default:
		fmt.Fprint(w, "terms   never snapshotted locally")
	}
	if t.PendingChangesets > 0 {
		noun := "change-sets"
		if t.PendingChangesets == 1 {
			noun = "change-set"
		}
		fmt.Fprintf(w, " · %d %s proposed, awaiting review", t.PendingChangesets, noun)
	}
	fmt.Fprintln(w)
	if t.PendingURL != "" {
		fmt.Fprintf(w, "        review: %s\n", t.PendingURL)
	}
}

// sourceLadder is the column order for the source-readiness line.
var sourceLadder = gate.SourceLadder()

// writeSourceLine renders the one-line source-readiness summary: per-rung
// coverage of the author's content (labeled, since its ladder differs from the
// translation grid) plus its source-gate standing. It uses the same verdict
// vocabulary as the ship column — a gate reads `ready` or `blocked: <rung>`
// wherever it appears.
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
		standing = " · ready"
	default:
		standing = " · blocked: " + sc.Pending[0].State
	}
	fmt.Fprintf(w, "source  %d units · %s%s\n\n", sc.Total, strings.Join(cells, " · "), standing)
}

// scopeLabel renders a coverage row's scope: the locale, or "locale/collection"
// when the row is collection-scoped.
func scopeLabel(lc LocaleCoverage) string {
	if lc.Collection != "" {
		return lc.Locale + "/" + lc.Collection
	}
	return lc.Locale
}

// shipCell renders the ship verdict: `ready`, `blocked: <rung>` naming the
// first unmet gate, or a dash when no gate applies to the scope.
//
// The blocking rung is the *lowest* unmet one (gate.Result.Blocking), so the
// verdict points at the work that unblocks the rest — saying "blocked: sign-off"
// while translation is also short would send someone to the wrong task.
func shipCell(lc LocaleCoverage, s *output.Styles) string {
	if !lc.Gated {
		return s.Dim("—")
	}
	if lc.Shippable {
		return s.Success.Render("ready")
	}
	return s.Warn.Render("blocked: " + shipBlockingLabel(lc))
}

// shipBlockingLabel names the blocking gate in the CLI's own vocabulary: the
// ladder rungs are lifecycle states ("signed-off"), but a verdict reads as the
// action that clears them ("sign-off").
func shipBlockingLabel(lc LocaleCoverage) string {
	blocking := lc.Blocking
	if blocking == "" && len(lc.Pending) > 0 {
		blocking = lc.Pending[0].State
	}
	switch blocking {
	case "":
		return "gate"
	case string(model.TargetStatusSignedOff):
		return "sign-off"
	case string(model.TargetStatusReviewed):
		return "review"
	case string(model.TargetStatusTranslated):
		return "translate"
	case string(model.TargetStatusDraft):
		return "draft"
	default:
		return blocking
	}
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
		out.Staged = a.PendingDecisions(ctxOrBackground(cmd.Context()), root)
		a.appendServerStatus(cmd, proj, &out)
		out.Venue = a.statusVenue(proj)
		a.WarnInertRecipeFields(cmd, proj)
		return output.Print(cmd, out)
	})
}

// appendServerStatus merges the connected-server delta into the status output
// when the recipe binds a convergence venue and the bowrain plugin is
// installed. It shells the plugin's `server-status --json` (subprocess
// dispatch — the cli module never imports bowrain) and folds the result under
// out.Server. Any failure degrades to a one-line stderr warning and leaves the
// local report intact: a status command must never fail on a server hiccup.
func (a *App) appendServerStatus(cmd Command, proj *project.KapiProject, out *StatusOutput) {
	if _, ok := proj.Venue(); !ok {
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
// nil for a recipe that binds none (no ambiguity to report). With one bound,
// the venue is "server" when a plugin provides the server-up plumbing `kapi up`
// dispatches to, otherwise a degraded "local" with a note.
func (a *App) statusVenue(proj *project.KapiProject) *StatusVenue {
	binding, ok := proj.Venue()
	if !ok {
		return nil
	}
	v := &StatusVenue{ConvergePolicy: binding.Converge}
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
