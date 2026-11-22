package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// ConvergeOptions carries the knobs of one convergence run (`kapi up` /
// no-argument `kapi run`).
type ConvergeOptions struct {
	// untilGate loops the pass until every gated scope ships (or a pass
	// stalls); false runs a single pass.
	UntilGate bool
	// maxPasses caps the loop.
	MaxPasses int
	// noExtract skips the pre-pass block-store drift check + auto-extract
	// (`up --no-extract`).
	noExtract bool
	// noChecks skips running the project's bound checks in the loop
	// (`up --no-checks`): produced units then count as translated even when
	// they would fail the guardrails.
	noChecks bool
	// materialize forces the post-loop materialize step (`up --materialize`)
	// regardless of the recipe's defaults.materialize policy.
	materialize bool
	// jobs is how many locales one pass runs concurrently (`up --jobs`);
	// <= 0 uses convergeJobsDefault. Locales are independent within a pass.
	jobs int
	// onEvent, when set, receives the run's convergence.Event stream: the
	// one protocol every surface renders a run from (CLI live view, NDJSON,
	// desktop run view, server SSE). Called from one goroutine at a time.
	onEvent func(convergence.Event)
	// capture, when non-nil, receives the final ConvergeOutput instead of it
	// being printed through the command's output formatter (embedders).
	capture *ConvergeOutput
	// recycleOnly replaces the project's flow with RecycleOnlyFlowSpec: the
	// pass fills what the content memory answers exactly and calls no
	// provider.
	recycleOnly bool
}

// ConvergeLocaleResult is the per-locale outcome of a convergence run.
type ConvergeLocaleResult struct {
	Locale    string         `json:"locale"`
	Shippable bool           `json:"shippable"`        // no scope for this locale is withheld
	Parked    bool           `json:"parked,omitempty"` // the loop left work here, waiting for review
	Pct       map[string]int `json:"pct,omitempty"`    // ladder state → "at least" percent
	// ShipState folds the locale's scopes into one standing: withheld when any
	// scope is withheld, shippable when every scope is gated and clears its gate,
	// and not_gated otherwise. A locale with content no ship gate matches makes
	// no shippable claim for it.
	ShipState ShipState `json:"ship_state"`
	// Gated reports whether a ship gate matches any of the locale's scopes.
	Gated bool `json:"gated"`
	// FailingChecks counts units that are produced but fail the project's bound
	// checks (#1078 G4). They count at their true rung in Pct (the unit is
	// translated) and hold the locale out of Shippable until fixed. It counts
	// UNITS, where `kapi check` lists findings, and one unit can carry several.
	FailingChecks int `json:"failing_checks,omitempty"`
	// Stale counts units whose translation was decided against source wording
	// that has since changed. They hold the locale out of Shippable whether or
	// not a gate applies: the decision on record no longer describes the
	// project, and only a person can replace it.
	Stale int `json:"stale,omitempty"`
	// Rejected counts units a reviewer turned down whose translation is still
	// the one they turned down. Their basis names the source the project holds,
	// so Stale does not count them, and they hold the locale out of Shippable
	// for the same reason: a person has said the wording will not do.
	Rejected int `json:"rejected,omitempty"`
	// Redrafted counts the stale units this run produced over: recycled against
	// the rewritten source, or drafted for the remainder. Each now holds a
	// translation of the current source. The decision they carried is NOT
	// restored, so they stay in Stale until someone reviews the new draft, and
	// the two are reported side by side.
	Redrafted int `json:"redrafted,omitempty"`
	// Materialized counts the target files written for this locale by the
	// post-loop materialize step (defaults.materialize: on-converge, or
	// --materialize). Only shippable locales materialize; a parked locale
	// stays at 0.
	Materialized int `json:"materialized,omitempty"`
	// StaleDrafts counts the blocks whose stored draft was made from an earlier
	// source, or written by an earlier kapi, which the materialize step
	// therefore did not write (blockstore.TargetOverlay.StaleFor). Stale counts
	// a decision whose basis moved, and Redrafted the units this run produced
	// over; a stale draft is neither, and stays until a pass with a producing
	// step drafts the block again.
	StaleDrafts int `json:"stale_drafts,omitempty"`
	// UnstampedDrafts names each file whose stored drafts in this locale an
	// earlier kapi wrote (blockstore.TargetOverlay.Unstamped). StaleDrafts
	// counts them too.
	UnstampedDrafts []output.UnstampedDrafts `json:"unstamped_drafts,omitempty"`
}

// ParkedScope identifies one gated (collection, locale) scope still short of
// its gate after the run: the address a review surface can deep-link to.
type ParkedScope struct {
	Locale     string `json:"locale"`
	Collection string `json:"collection,omitempty"`
}

// ConvergeOutput is the structured result of `kapi run` driving the default
// flow over a project's content. One pass by default; looped to the ship gate
// under --until-gate.
type ConvergeOutput struct {
	Flow      string                 `json:"flow"`
	Passes    int                    `json:"passes"`
	Converged bool                   `json:"converged"` // no scope is withheld: every gated scope clears its gate
	Locales   []ConvergeLocaleResult `json:"locales"`
	// Produced counts the units this run wrote to the project's files, by its
	// passes or by the delivery after its loop, from this run's drafts or an
	// earlier run's. A run that changed nothing reports 0. Nil when the venue
	// does not report it: a server run writes to the server, and its summary
	// leaves the figure out rather than give it another meaning.
	Produced *int `json:"produced,omitempty"`
	// WaitingForReview counts the drafts the run holds for review in locales
	// short of their ship gate. They are not written to the project, so
	// Produced leaves them out.
	WaitingForReview int `json:"waiting_for_review,omitempty"`
	// ParkedScopes lists the gated (collection, locale) scopes that remain
	// short of their gate: per-scope detail under the per-locale rollup, so
	// a UI can link each parked scope to its review queue.
	ParkedScopes []ParkedScope `json:"parked_scopes,omitempty"`
	// MaterializedFiles is the total count of target files written by the
	// post-loop materialize step across every shippable locale (0 when the
	// policy is manual and --materialize was not passed).
	MaterializedFiles int `json:"materialized_files,omitempty"`
	// BlockedOnSource is how many translatable source blocks were held below the
	// translate_after level this run: their translations were not produced
	// because the source is un-settled. 0 when the level is `none` or the
	// source is fully settled. It is source-scoped (deduped across locales, as
	// a source block is shared by every target), mirroring the server's run row.
	BlockedOnSource int `json:"blocked_on_source,omitempty"`
	// TranslateAfter is the resolved defaults.translate_after level applied
	// (none|written|established), for observability. Empty when no level
	// was evaluated (no content).
	TranslateAfter string `json:"translate_after,omitempty"`
	// StallReason is the machine-readable cause a run did not converge. It is
	// source_not_ready when every pending locale had nothing producible because
	// its source is held below the translate_after level. Empty on a
	// clean/parked-on-target run.
	StallReason string `json:"stall_reason,omitempty"`
	// Monolingual reports a run over a project that resolves no target locale at
	// all: the source half of convergence ran and the per-locale fan-out was not
	// applicable. Locales is empty for such a run, and a reader needs to tell
	// that apart from a multilingual project whose locales all dropped out.
	Monolingual bool `json:"monolingual,omitempty"`
	// NoTargets reports a run over a project that declares target languages but
	// has no collection with a target path: the source half ran, and no
	// translation had anywhere to go.
	NoTargets bool `json:"no_targets,omitempty"`
	// ExtractedFiles and ExtractedBlocks report the pre-pass re-extraction that
	// brought the project store back in sync with the working tree. Both 0 when
	// the store was already current. They are the "what moved" of a monolingual
	// run, whose whole output is otherwise source-side.
	ExtractedFiles  int `json:"extracted_files,omitempty"`
	ExtractedBlocks int `json:"extracted_blocks,omitempty"`
	// ConceptsProposed, ChangesetID and ChangesetURL report governed terminology
	// the run proposed rather than applied: edits that take effect only once a
	// reviewer accepts them. A venue that governs terminology fills them in,
	// and all three are zero for a run that proposed nothing.
	//
	// ChangesetURL is a deep link to the review surface, and is empty whenever
	// the venue could not name one. The id is reported either way: it is the
	// handle every other surface takes, so a reader who cannot click through
	// can still ask for the change-set by name.
	ConceptsProposed int    `json:"concepts_proposed,omitempty"`
	ChangesetID      string `json:"changeset_id,omitempty"`
	ChangesetURL     string `json:"changeset_url,omitempty"`
	// Warnings name what the run could not take in, such as a declared file in
	// a format no installed reader opens (check.WarningFormatNoReader). The run
	// set that content aside and converged the rest.
	Warnings []check.Warning `json:"warnings,omitempty"`
	// Findings holds the failing findings of a program step whose output
	// changed a redaction placeholder (tools.RulePlaceholder). Each such block
	// was left as it was, and the run exits with the code a failed check uses.
	// Nil when there are none.
	Findings *FindingsReport `json:"findings,omitempty"`
	// PreReview counts what the AI pre-review scored after the run, nil when
	// none was asked for (kapi up --pre-review).
	PreReview *PreReviewResult `json:"pre_review,omitempty"`
}

// StaleDraftUnits totals the blocks whose stored draft was made from an
// earlier source and was not written, across every locale of the run.
func (o ConvergeOutput) StaleDraftUnits() int {
	n := 0
	for _, lc := range o.Locales {
		n += lc.StaleDrafts
	}
	return n
}

// StaleUnits totals the units held out of shipping because their source moved
// since the translation was decided, across every locale of the run.
func (o ConvergeOutput) StaleUnits() int {
	n := 0
	for _, lc := range o.Locales {
		n += lc.Stale
	}
	return n
}

// RejectedUnits totals the units a reviewer turned down that still hold the
// refused wording after the run, across every locale.
func (o ConvergeOutput) RejectedUnits() int {
	n := 0
	for _, lc := range o.Locales {
		n += lc.Rejected
	}
	return n
}

// RedraftedUnits totals the stale units this run produced a fresh translation
// for, across every locale.
func (o ConvergeOutput) RedraftedUnits() int {
	n := 0
	for _, lc := range o.Locales {
		n += lc.Redrafted
	}
	return n
}

// FormatText renders the convergence summary.
func (o ConvergeOutput) FormatText(w io.Writer) error {
	defer writeSetAside(w, o.Warnings)
	defer o.PreReview.formatLine(w)
	if o.Monolingual || o.NoTargets {
		err := o.formatMonolingual(w)
		o.formatPlaceholderFindings(w)
		return err
	}
	verb := "pass"
	if o.Passes != 1 {
		verb = "passes"
	}
	fmt.Fprintf(w, "Ran flow %q over %d locale(s) in %d %s.\n\n", o.Flow, len(o.Locales), o.Passes, verb)
	if o.Produced != nil {
		fmt.Fprintf(w, "  produced            %d\n", *o.Produced)
	}
	if o.WaitingForReview > 0 {
		fmt.Fprintf(w, "  waiting for review  %d\n", o.WaitingForReview)
	}
	fmt.Fprintln(w)
	t := output.NewTable(w).Accent(0).
		Headers("locale", "drafted", "translated", "state", "checks")
	s := t.Styles()
	for _, lc := range o.Locales {
		state := s.Warn.Render("pending")
		switch {
		case lc.Parked:
			state = s.Warn.Render("waiting for review")
		case lc.ShipState == ShipStateNotGated:
			// No ship gate matches the locale's content, so there is no bar the
			// run could report it as clearing.
			state = s.Dim("not gated")
		case lc.Shippable:
			state = s.Success.Render("✓ shippable")
		}
		// Units, named as units. `kapi check` counts finding rows over the same
		// tree and one unit can carry several, so two numbers that were both
		// labelled "check(s)" read as a contradiction between the commands.
		checks := s.Dim("")
		if lc.FailingChecks > 0 {
			checks = s.Error.Render(fmt.Sprintf("%d block(s) failing checks", lc.FailingChecks))
		}
		t.Row(lc.Locale,
			fmt.Sprintf("%d%%", lc.Pct["draft"]),
			fmt.Sprintf("%d%%", lc.Pct["translated"]),
			state, checks)
	}
	t.Render()
	fmt.Fprintln(w)
	// Source hold: when translate_after held blocks below its level, say so.
	// Those blocks were NOT translated, and the fix is to settle the source
	// rather than to wait for review.
	if o.BlockedOnSource > 0 {
		fmt.Fprintf(w, "%d block(s) held on source. Settle your source first "+
			"(kapi check --ship, or fix terms/brand/source and kapi apply), then re-run.\n", o.BlockedOnSource)
	}
	// Source drift under a decided translation, and the two halves of settling
	// it. The loop owns the first: a translation of the source the project has
	// now. The second belongs to a person, whose decision was withdrawn, so the
	// two are reported together. These lines keep drift from reading as
	// converged, and the re-draft line shows the loop worked on it.
	if redrafted := o.RedraftedUnits(); redrafted > 0 {
		fmt.Fprintf(w, "Re-drafted %d stale block(s) against the current source. "+
			"They return to review un-approved (the earlier approval is not restored).\n", redrafted)
	}
	if stale := o.StaleUnits(); stale > 0 {
		fmt.Fprintf(w, "%d block(s) stale: their source changed since the translation was decided. "+
			"Re-review them (kapi status --review); they do not ship until you do.\n", stale)
	}
	// A refusal the run could not answer. The wording a reviewer turned down is
	// still in the file, so the locale is held whatever the percentages say, and
	// the reader is told which half of the loop is stuck: production reached the
	// unit and produced the same words, or the flow has no step that would.
	if rejected := o.RejectedUnits(); rejected > 0 {
		fmt.Fprintf(w, "%d block(s) still hold wording a reviewer turned down. "+
			"They do not ship until a pass drafts something else for them.\n", rejected)
	}
	if o.Findings != nil {
		o.formatPlaceholderFindings(w)
	} else if o.Converged {
		fmt.Fprintln(w, o.upToDateLine())
	} else if o.StallReason == convergence.StallSourceNotReady {
		fmt.Fprintln(w, "Held on source. Nothing translatable is settled yet. Set defaults.translate_after: none to draft freely.")
	} else {
		fmt.Fprintln(w, "Not yet up to date: some languages wait for review (never a build failure).")
	}
	if o.MaterializedFiles > 0 {
		fmt.Fprintf(w, "Materialized %d target file(s) from the project store.\n", o.MaterializedFiles)
	}
	if staleDrafts := o.StaleDraftUnits(); staleDrafts > 0 {
		fmt.Fprintf(w, "Note: %d block(s) hold a stale draft (an earlier source, or an earlier kapi) and were not written.\n", staleDrafts)
		for _, lc := range o.Locales {
			for _, u := range lc.UnstampedDrafts {
				fmt.Fprintf(w, "  %s\n", u.Line())
			}
		}
	}
	o.formatProposedChangeset(w)
	return nil
}

// formatPlaceholderFindings prints the placeholder findings and the line that
// says the run is not up to date because of them. It prints nothing when the
// run has none.
func (o ConvergeOutput) formatPlaceholderFindings(w io.Writer) {
	if o.Findings == nil {
		return
	}
	o.Findings.FormatTable(w)
	fmt.Fprintf(w, "Not up to date: %d block(s) left unchanged because a program's output changed a redaction placeholder.\n", placeholderBlocks(*o.Findings))
}

// upToDateLine is the closing line of a converged run. It claims the gates only
// where a gate matches: a project that declares none is up to date with nothing
// reported shippable, and a language no gate matches is named as not gated.
func (o ConvergeOutput) upToDateLine() string {
	var notGated []string
	gated := false
	for _, lc := range o.Locales {
		if lc.ShipState == ShipStateNotGated {
			notGated = append(notGated, lc.Locale)
		}
		if lc.Gated || lc.ShipState != ShipStateNotGated {
			gated = true
		}
	}
	switch {
	case len(o.Locales) > 0 && !gated:
		return "Up to date. No ship gates are declared."
	case len(notGated) > 0:
		return "Up to date: every gated locale is shippable. Not gated: " + strings.Join(notGated, ", ") + "."
	default:
		return "Up to date: every gated locale is shippable."
	}
}

// formatProposedChangeset reports governed terminology the run proposed rather
// than applied, and says where to review it.
//
// The link is the point. A run that ends "proposed 57 governed terminology
// edit(s) in change-set EMezX9AS" names something the reader has no way to
// reach: the id is a handle for a surface they have to go and find. Where the
// venue named a review URL, print it; where it did not, the id alone is still
// the honest report, and better than a link built from a guess at the host.
func (o ConvergeOutput) formatProposedChangeset(w io.Writer) {
	if o.ConceptsProposed == 0 {
		return
	}
	fmt.Fprintf(w, "Proposed %d governed terminology edit(s) in change-set %s. They take effect when reviewed.\n",
		o.ConceptsProposed, o.ChangesetID)
	if o.ChangesetURL != "" {
		fmt.Fprintf(w, "Review it at %s\n", o.ChangesetURL)
	}
}

// formatMonolingual renders a run over a project with no target locale. It
// reports the store, because the whole source half of the run (seed, extract,
// occurrence graph) completed, and an empty locale grid would read as
// "nothing was reconciled".
func (o ConvergeOutput) formatMonolingual(w io.Writer) error {
	if o.NoTargets {
		fmt.Fprintf(w, "Reconciled the source. No collection has a target path, so the per-language flow %q wrote no translations. Add `target:` to a collection in kapi.yaml to say where its translations go.\n\n", o.Flow)
	} else {
		fmt.Fprintf(w, "Reconciled the source. No target languages are configured, so the per-language flow %q did not run.\n\n", o.Flow)
	}
	if o.ExtractedBlocks > 0 || o.ExtractedFiles > 0 {
		fmt.Fprintf(w, "Extracted %d block(s) from %d file(s) into the project store.\n", o.ExtractedBlocks, o.ExtractedFiles)
	} else {
		fmt.Fprintln(w, "The project store already matched the working tree.")
	}
	fmt.Fprintln(w, "Up to date: the committed context and the occurrence graph match the sources.")
	o.formatProposedChangeset(w)
	return nil
}

// RunDefaultFlowConverge executes the project's default flow (defaults.flow)
// over its content across every target language. Without untilGate it runs a
// single pass; with untilGate it loops the pass, re-deriving coverage each
// time, until every gated locale is shippable, a pass makes no progress, or
// maxPasses is reached, and parks the locales that remain short of their
// gate. It never fails the build: parked work is reported as such, because
// target drift is normal toil.
func (a *App) RunDefaultFlowConverge(cmd Command, proj *project.KapiProject, projectPath string, opts ConvergeOptions) error {
	untilGate, maxPasses := opts.UntilGate, opts.MaxPasses
	cf, err := convergeFlowSpec(proj, filepath.Dir(projectPath), runFlagToolConfig(cmd))
	if err != nil {
		return err
	}
	PrintRunConfigNotes(cmd, cf.notes)
	if opts.recycleOnly {
		cf.name, cf.label = recycleOnlyFlowName, recycleOnlyFlowName
		cf.spec = RecycleOnlyFlowSpec()
		if cf.translateAfter != model.TranslateAfterNone {
			cf.spec = prependTranslateAfter(cf.spec, cf.translateAfter)
		}
	}
	flowName, flowLabel, spec, translateAfter := cf.name, cf.label, cf.spec, cf.translateAfter

	ctx := CmdContext(cmd)

	// Project context + content sources (the flow reads source, writes per-locale
	// targets via the project's target template).
	pctx := project.NewProjectContext(proj, projectPath)
	// The one place the recipe's source language is adopted for a convergence:
	// an explicit --source-lang still wins, and a project that declares none
	// leaves the built-in default standing. A pass that reaches the loop at the
	// wrong language misses every content-memory lookup and re-drafts the whole
	// project over its own approved wording (host/sourcelang.go).
	a.ResolveSourceLang(pctx.SourceLocale)
	// And the encoding it reads them in, for the same reason: the writer already
	// takes the recipe's (project.ConfigureWriterFor), so a reader left on the
	// flag's own value decodes and re-encodes a project through two different
	// character sets (host/encoding.go).
	a.ResolveEncoding(pctx.Encoding)
	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return fmt.Errorf("resolve content: %w", err)
	}
	// A file declared for its comments alone is checked, and has nothing a
	// convergence run could read, translate or write back.
	//
	// A file whose declared format no installed reader opens, usually a
	// plugin's, is set aside: the run converges the rest and names it.
	// --fail-on-unknown keeps it in, and the run fails on it.
	var unread *UnreadSet
	if !failOnUnknown(cmd) {
		unread = a.newUnreadSetFor("converged")
	}
	var sources []string
	kept := resolved[:0]
	for _, rf := range resolved {
		if rf.CommentsOnly() || a.setAside(unread, filepath.Dir(projectPath), rf) {
			continue
		}
		kept = append(kept, rf)
		// A file whose item names no target has no destination in any
		// language. It extracts and settles with the rest of the source, and
		// no locale pass runs over it.
		if rf.Item == nil || rf.Item.Target == "" {
			continue
		}
		sources = append(sources, rf.Path)
	}
	resolved = kept
	if len(resolved) == 0 {
		if !unread.empty() {
			// Nothing a run could read is nothing converged, never a run that
			// found every scope shippable.
			return fmt.Errorf("nothing was converged: %s", unread.summary())
		}
		return errors.New("no content to catch up (add content patterns to the project)")
	}

	// The locales this run fans out over, and the verdict that decides whether it
	// fans out at all. A project that resolves none is MONOLINGUAL, a supported
	// shape: the source half of convergence still runs (the committed context
	// seeds, the working tree re-extracts into the store, the occurrence graph
	// refreshes) and only recycle/translate/materialize do not apply. A
	// one-language project compiles its own sources into the store this way.
	locales := contentTargetLocales(proj.Defaults, resolved)
	monolingual := len(locales) == 0

	// The committed translations, before anything resolves out of the store:
	// the target documents git carries are content, and the wording a reviewer
	// approved in them supersedes whatever a provider produced. A run that
	// skipped them would spend a provider re-translating what the working tree
	// already answers, so a failure here is PROPAGATED.
	//
	// A run that prints its change set writes nothing into the content memory:
	// its pass draws on the memory as it stands.
	if a.printOps == nil {
		absorbed, aerr := a.absorbProjectRecord(ctx, proj, projectPath)
		if aerr != nil {
			return fmt.Errorf("absorb the committed record: %w", aerr)
		}
		if line := formatRecordLine(absorbed); line != "" && !a.Quiet {
			fmt.Fprintln(cmd.ErrOrStderr(), line)
		}
	}

	// Standing project context + bindings, so flow steps honor the voice
	// profile and terms and write to the right per-locale target paths.
	a.ProjectContext = pctx
	defer func() { a.ProjectContext = nil }()
	bindings, err := a.resolveProjectBindings(cmd, proj, projectPath, a.GovernancePointFor("", ""))
	if err != nil {
		return err
	}
	// The loop itself keeps the project-wide bindings: they feed the coverage
	// derivation and the check gates, which are not per collection.
	a.ProjectBindings = bindings
	defer func() { a.ProjectBindings = nil }()

	// This loop runs here, on this machine, and resolves the recipe's
	// coordinates; the same loop at the server venue would not, until they are
	// synced there.
	a.WarnUnsyncedCoordinates(cmd.ErrOrStderr(), proj)

	// The flow run inside a pass is per collection: the source set splits into
	// one group per distinct (voice profile, terms) binding, and a worker runs the
	// flow once per group. A recipe where no collection overrides either yields
	// one group holding every source, at the project's default point, so its
	// bindings are the project-wide ones and the pass is the single flow run per
	// locale convergence has always done.
	groups, err := a.groupInputsByBinding(cmd, proj, pctx.ProjectDir, sources)
	if err != nil {
		return err
	}
	// Bindings are resolved at (group point, locale) inside the fan-out below,
	// because the term rules in them are per target locale. A concept's approved
	// wording differs between nb and fr, and no locale is known here yet.
	groupBindings := a.newLocaleBindings(cmd, proj, projectPath)

	root := filepath.Dir(projectPath)
	absProjectPath, _ := filepath.Abs(projectPath)
	projectDir := filepath.Dir(absProjectPath)

	savedTarget := a.TargetLang
	defer func() { a.TargetLang = savedTarget }()

	// Convergence writes real localized files (not just block-store overlays) so
	// its file-derived coverage sees each pass's output, uniformly across
	// single- and multi-file projects.
	a.convergeWriteFiles = true
	defer func() { a.convergeWriteFiles = false }()

	// Under a policy that gates delivery, those files are drafts. The pass
	// writes them into the run's own tree, and the gated materialize in
	// finishConverge is the only write that reaches a collection's `target:`
	// path. That keeps the promise `on-converge` makes: a locale short of its
	// ship gate gets no files, and no unreviewed draft sits where a site build
	// globs (#1936). See host/convergedrafts.go.
	//
	// A run that prints its change set drafts into the same tree, so its gate
	// evaluates the same output, and delivery prints what it would commit.
	if deliveryIsGated(proj, opts) {
		endDrafts, derr := a.beginConvergeDrafts(projectPath, projectDir)
		if derr != nil {
			return derr
		}
		defer endDrafts()
	}

	if maxPasses < 1 {
		maxPasses = 1
	}
	jobs := opts.jobs
	if jobs <= 0 {
		jobs = convergeJobsDefault
	}

	onEvent := opts.onEvent
	emitter := convergence.NewEmitter(onEvent)
	facts := &convergeFacts{monolingual: monolingual, noTargets: !monolingual && len(sources) == 0, unread: unread}
	if a.specChecksRedactionTokens(spec) {
		// A program step under a redaction policy records a failing finding on
		// each block whose program output changes a placeholder, and the block
		// stays as it was. The passes collect those findings, and the run
		// reports them and fails (placeholderGateError).
		facts.placeholders = &flowFindings{}
	}
	announceSetAside(emitter, unread)
	if onEvent == nil {
		// No event consumer: the set-aside lines go to stderr instead.
		unread.warn(a, cmd)
	}

	// The venue-neutral loop (core/convergence.Loop) owns the semantics:
	// pass barrier, per-locale fan-out, stall-parks-the-rest; these closures
	// are the CLI venue's IO: working-tree drift re-extract, file-derived
	// coverage with bound checks, and the default flow on per-locale worker
	// Apps.
	//
	//
	// Produced is the run's tally of the translations it wrote to target
	// files (unitWriteTally), counted where the change service records each
	// write. Under gated delivery a pass drafts into the draft tree and writes
	// no target file, and a withheld locale's drafts are counted as waiting
	// for review instead.
	gatedDelivery := deliveryIsGated(proj, opts)
	tally := a.writeTally
	a.writeTally = &unitWriteTally{}
	defer func() { a.writeTally = tally }()
	var productionMu sync.Mutex
	production := map[string]convergence.PassProduction{}
	derive := func(cov []LocaleCoverage, excl *CheckExclusions, unstamped map[string]int) convergence.PassState {
		pending := localesNeedingWork(cov, locales, unstamped)
		facts.noteRedraftable(cov, pending)
		waiting := 0
		if gatedDelivery {
			productionMu.Lock()
			held := heldDrafts(cov, production)
			productionMu.Unlock()
			waiting = sumCounts(held)
			facts.held = held
		}
		return convergence.PassState{
			Pending:          localeStrings(pending),
			Produced:         a.writeTally.total(),
			WaitingForReview: waiting,
			FailingChecks:    excl.totalFailing(),
			UnitTotals:       localeUnitTotals(cov),
			Detail:           derivedState{cov: cov, excl: excl},
		}
	}
	funcs := convergence.LoopFuncs{
		Derive: func(ctx context.Context) (convergence.PassState, error) {
			cov, excl, err := a.deriveCoverage(ctx, cmd, proj, root, !opts.noChecks, unread)
			if err != nil {
				return convergence.PassState{}, err
			}
			unstamped, err := a.unstampedDrafts(ctx, root, locales)
			if err != nil {
				return convergence.PassState{}, err
			}
			return derive(cov, excl, unstamped), nil
		},
		Produce: func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (convergence.PassProduction, error) {
			tap := newConvergeTap(locale)
			worker := a.convergeWorker(locale, tap)
			stopWatch := watchTapProgress(tap, pass, emit.Emit)
			rCtx := flow.ResourceContext{ProjectDir: projectDir, SourceLocale: worker.SourceLang, TargetLocale: locale}
			// One flow run per binding group, sequentially on this locale's
			// worker: each group's bindings, resolved for this locale, are on
			// the worker before its tool chain is assembled. The tap counts
			// across all of them, so the locale's progress is unaffected by how
			// the sources split.
			var err error
			for _, group := range groups {
				b, berr := groupBindings.at(group.Point, locale)
				if berr != nil {
					err = berr
					break
				}
				worker.ProjectBindings = b
				worker.flowRedaction = &group.Redaction
				// A quiet worker arms no findings collector of its own
				// (beginFlowFindings), so the run's placeholder collector
				// stays armed and taps the worker's chain per file.
				worker.flowFindings = facts.placeholders
				if err = worker.runProjectStepsOver(ctx, cmd, flowName, spec, &rCtx, group.Inputs); err != nil {
					break
				}
			}
			stopWatch()
			if err != nil {
				return convergence.PassProduction{}, err
			}
			made := tap.snapshot()
			productionMu.Lock()
			production[locale] = made
			productionMu.Unlock()
			return made, nil
		},
	}
	if !opts.noExtract {
		// Auto-extract on drift (#1078 C2): before each pass, bring the
		// project block store back in sync with the working tree. A missing
		// store, a version-stamp mismatch, or edited source files all trigger
		// a re-extract through the same shared path the desktop's Re-extract
		// uses. `up --no-extract` opts out.
		funcs.Sync = func(ctx context.Context) (*convergence.SyncResult, error) {
			stats, reason, serr := a.syncProjectBlockStore(ctx, pctx, projectPath, resolved)
			if serr != nil {
				return nil, fmt.Errorf("sync project block store: %w", serr)
			}
			if stats == nil {
				return nil, nil
			}
			facts.extractedFiles += stats.Files
			facts.extractedBlocks += stats.Blocks
			// Stamp-write failures do not endanger the extracted content, so
			// they never fail the pass. Unreported, they would make kapi
			// re-extract and re-translate everything on every run with no
			// visible cause. The warning is independent of the event consumer
			// (an operator fault, not run progress) and respects --quiet; a
			// caller reading ExtractStats gets them in Warnings either way.
			if !a.Quiet {
				for _, w := range stats.Warnings {
					fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+w)
				}
			}
			if opts.onEvent == nil && !a.Quiet {
				// No event consumer: keep the plain run-log line (bare
				// `kapi run`, embedders listening on LogWriter only).
				fmt.Fprintf(cmd.OutOrStdout(), "Extracted %d block(s) from %d file(s) into the project store (%s).\n",
					stats.Blocks, stats.Files, reason)
			}
			return &convergence.SyncResult{Files: stats.Files, Blocks: stats.Blocks, Reason: reason}, nil
		}
	}

	// Admission: every destination this run would write must be writable
	// (#1449). A blocked path (most often a directory standing where the writer
	// needs a file) is an operational fault, and a run that cannot write its
	// outputs does not start. Starting would fail the locale mid-pass and throw
	// away the other locales' in-flight work and the AI spend behind it.
	//
	// The resolve error is PROPAGATED. UnitsFromProject is a pure re-resolution
	// of the recipe's content patterns, and the loop's own Derive
	// (deriveCoverage) calls it identically and fails a moment later, so
	// failing here names the fault sooner.
	admissionUnits, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return fmt.Errorf("resolve project content: %w", err)
	}
	if berr := CheckTargetPathsWritable(admissionUnits); berr != nil {
		return berr
	}
	// The source each translation on disk was made from is recorded in the
	// project's context, which a fresh checkout has not read yet.
	a.noteHistoryNotPulled(cmd, root, admissionUnits)

	// A producer serves a draft the workspace home keeps from its overlay in
	// the block store, a cache a deleted `.kapi/work/` loses: write back the
	// overlays the store lacks before a pass asks for them, so a kept draft
	// costs no provider call to serve again.
	if _, rerr := a.restoreKeptOverlays(cmd.Context(), root); rerr != nil {
		return fmt.Errorf("restore the drafts the workspace home keeps: %w", rerr)
	}

	// Share one parse cache across every pass: unchanged source files parse once,
	// not once per pass; only the targets a pass rewrites re-parse.
	return a.withParseCache(root, func() error {
		// Source-first settle phase (epic 019): settle the source ONCE before the
		// loop, stamp SourceStatus over the source files, and count the blocks
		// held below the translate_after level ("N blocks held on source"). A
		// no-op when the level is `none`.
		//
		// The settle error is PROPAGATED. Dropped, it would leave blockedOnSource
		// and totalSource at 0, and the run would read as converged with its
		// materialize guard disarmed.
		//
		// admissionUnits, resolved above, lists the files that feed a target
		// language, which is what the hold is measured over. The settle itself
		// covers the whole source, including a source-only collection.
		var blockedOnSource, totalSource int
		if translateAfter != model.TranslateAfterNone {
			srcUnits, serr := a.SourceUnitsFromProject(proj, root)
			if serr != nil {
				return fmt.Errorf("resolve source content: %w", serr)
			}
			settled, herr := a.settleAndCountHeldSource(ctx, root, a.SourceLang, translateAfter, srcUnits, admissionUnits)
			if herr != nil {
				return fmt.Errorf("settle source for translate_after %q: %w", translateAfter, herr)
			}
			held, unreadable := settled.held, settled.unreadable
			blockedOnSource, totalSource = held, settled.heldOf
			// A collection whose format has no reader on this machine was never
			// opened, so it contributed nothing to the two counts above. That is
			// survivable (the plugin supplying it is optional), but it is not
			// silent: a hold that reports "0 held" over content it could not read
			// is indistinguishable from one that read it and found it clean.
			for _, f := range unreadable {
				emitter.Emit(convergence.Event{
					Type:    convergence.EventLog,
					Stage:   convergence.StageSettleSource,
					Message: translateAfterUnreadMessage(a.discoveredPlugins(), f),
				})
			}
			emitter.Emit(convergence.Event{
				Type:            convergence.EventLog,
				Stage:           convergence.StageSettleSource,
				SettledSource:   len(settled.states),
				BlockedOnSource: held,
				Message:         settleSourceLine(settled, translateAfter),
			})
		}

		res, err := convergence.Loop(ctx, convergence.LoopOptions{
			UntilGate: untilGate,
			MaxPasses: maxPasses,
			Jobs:      jobs,
		}, funcs, emitter)
		if err != nil {
			return a.failedAfterWrites(cmd, err)
		}
		// The run read the whole source, so how it writes each suggestion's
		// forms is evidence for and against the suggestion (core/contextop).
		if !opts.noChecks {
			a.countProjectUsage(ctx, cmd, projectPath, resolved)
		}
		d := res.Final.Detail.(derivedState)
		facts.waitingForReview = res.Final.WaitingForReview
		if err := a.finishConverge(ctx, cmd, proj, projectPath, flowLabel, res.Passes, d.cov, locales,
			translateAfter, blockedOnSource, totalSource, facts, opts, emitter.Emit); err != nil {
			return a.failedAfterWrites(cmd, err)
		}
		if r := facts.placeholderReport(); r != nil {
			return placeholderGateError(*r)
		}
		return nil
	})
}

// failedAfterWrites is err for a run that failed after it may have written
// target files. It carries how many translation units the run wrote, which
// --json prints beside the error (ConvergeFailedError); a text run says so on
// stderr when it wrote any.
func (a *App) failedAfterWrites(cmd Command, err error) error {
	n := a.writeTally.total()
	if n > 0 && output.ResolveFormat(cmd) != output.FormatJSON {
		fmt.Fprintf(cmd.ErrOrStderr(), "kapi up: wrote %d translation(s) to target files before it failed\n", n)
	}
	return &ConvergeFailedError{Err: err, Produced: n}
}

// ConvergeFailedError is a convergence run that failed, carrying how many
// translation units it wrote to target files first, so a --json caller reads
// the count beside the error.
type ConvergeFailedError struct {
	Err      error
	Produced int `json:"produced"`
}

func (e *ConvergeFailedError) Error() string { return e.Err.Error() }
func (e *ConvergeFailedError) Unwrap() error { return e.Err }

// ErrorFields is the count --json prints beside the error.
func (e *ConvergeFailedError) ErrorFields() any {
	return struct {
		Produced int `json:"produced"`
	}{e.Produced}
}

// convergeFacts are the run-scoped facts finishConverge reports that the
// coverage rollup cannot derive: whether the project fanned out at all, what the
// pre-pass extraction moved, and the work the run took in.
type convergeFacts struct {
	monolingual     bool
	noTargets       bool
	extractedFiles  int
	extractedBlocks int
	// waitingForReview is the drafts the run holds for review
	// (convergence.LoopResult).
	waitingForReview int
	// held is the units the run holds for review, per locale, as the last
	// derivation counted them (heldDrafts).
	held map[string]int
	// redraftable is, per locale, how many stale units the run STARTED with in
	// scopes it then fanned out over. The final rollup cannot recover it: a
	// re-drafted unit is still stale (its basis names source that is gone, and
	// producing a translation does not decide it), so the count that says "the
	// loop did its half" has to be taken before the half is done.
	redraftable map[string]int
	// unread names the content the run set aside because no installed reader
	// opens its format. Nil when --fail-on-unknown keeps it in.
	unread *UnreadSet
	// placeholders collects, over every pass, the findings of a program step
	// whose output changed a redaction placeholder. Nil when the flow holds no
	// such step behind a redact step.
	placeholders *flowFindings
}

// placeholderReport is the findings report of the placeholder findings the
// passes collected, or nil when there are none.
func (f *convergeFacts) placeholderReport() *FindingsReport {
	if f == nil || f.placeholders == nil {
		return nil
	}
	all := f.placeholders.report()
	var diags []check.Diagnostic
	for _, d := range all.Findings {
		if d.Rule == placeholderRule {
			diags = append(diags, d)
		}
	}
	if len(diags) == 0 {
		return nil
	}
	r := newFindingsReport(check.BuildReport(all.Target, diags))
	return &r
}

// announceSetAside emits a run log event for each format no installed reader
// opens, naming its files and the plugin to install, the way the source settle
// names a format it could not read.
func announceSetAside(emitter *convergence.Emitter, unread *UnreadSet) {
	formats, files := unread.byFormat()
	for _, format := range formats {
		emitter.Emit(convergence.Event{
			Type:  convergence.EventLog,
			Stage: convergence.StageSync,
			Message: fmt.Sprintf("No reader for format %q: %s set aside and not converged. %s.",
				format, named(files[format]), sentenceCase(installClause(unread.readers[files[format][0]].plugin))),
		})
	}
}

// noteRedraftable records the first pass's stale-unit count for the locales that
// pass will fan out over. Only the first derivation counts: later passes see the
// units the run has already produced over, and crediting them again would report
// one unit re-drafted once per pass.
func (f *convergeFacts) noteRedraftable(cov []LocaleCoverage, pending []model.LocaleID) {
	if f.redraftable != nil {
		return
	}
	f.redraftable = map[string]int{}
	running := make(map[string]bool, len(pending))
	for _, loc := range pending {
		running[string(loc)] = true
	}
	for _, c := range cov {
		if c.Stale > 0 && running[c.Locale] {
			f.redraftable[c.Locale] += c.Stale
		}
	}
}

// contentTargetLocales is the set of target locales a project's resolved content
// expands to: the recipe's `defaults.target_languages`, plus any a content item
// declares for itself. Empty means monolingual.
//
// It resolves exactly as UnitsFromProject does (same per-item fallback, same
// skip of an item with no target template), so the locales a run fans out over
// and the units its coverage is derived from agree, including for a recipe
// that names its languages per item.
func contentTargetLocales(defaults project.Defaults, resolved []project.ResolvedFile) []model.LocaleID {
	seen := make(map[model.LocaleID]bool, len(defaults.TargetLanguages))
	out := make([]model.LocaleID, 0, len(defaults.TargetLanguages))
	add := func(loc model.LocaleID) {
		if loc == "" || seen[loc] {
			return
		}
		seen[loc] = true
		out = append(out, loc)
	}
	for _, loc := range defaults.TargetLanguages {
		add(loc)
	}
	for _, rf := range resolved {
		if rf.Item == nil || rf.Item.Target == "" {
			continue
		}
		for _, loc := range rf.Item.ResolvedTargetLanguages(nil, defaults) {
			add(loc)
		}
	}
	return out
}

// derivedState is the CLI venue's rich derivation, threaded through the loop's
// PassState.Detail to finishConverge.
type derivedState struct {
	cov  []LocaleCoverage
	excl *CheckExclusions
}

// localeStrings converts locale IDs for the event stream.
func localeStrings(locales []model.LocaleID) []string {
	if len(locales) == 0 {
		return nil
	}
	out := make([]string, len(locales))
	for i, loc := range locales {
		out[i] = string(loc)
	}
	return out
}

// localeUnitTotals sums each locale's unit count across its coverage scopes:
// the denominator its live progress renders against.
func localeUnitTotals(cov []LocaleCoverage) map[string]int {
	totals := make(map[string]int)
	for _, c := range cov {
		totals[c.Locale] += c.Total
	}
	return totals
}

// syncProjectBlockStore detects block-store drift against the working tree and
// re-extracts the project's content into the store when any is found: nothing
// extracted yet, stamped by a different kapi, or source files whose bytes
// drifted from their extract-time stamps. Extraction goes through the shared
// host path (ExtractToProjectStore), the same implementation behind the
// desktop's Re-extract, and is a full rebuild of the block set and of the
// context graph projected from it (blocks are a pure cache; target overlays
// are preserved). No drift → no work beyond cheap stat checks. The returned
// reason describes the drift for run logs; reporting is the caller's job
// (event stream or plain print).
func (a *App) syncProjectBlockStore(ctx context.Context, pctx *project.ProjectContext, projectPath string, resolved []project.ResolvedFile) (*project.ExtractStats, string, error) {
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return nil, "", err
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return nil, "", err
	}
	drift := db.DetectStoreDrift(ctx, resolved)
	if !drift.Any() {
		return nil, "", nil
	}
	// The shared path rebuilds the block set and then the context graph it
	// projects, so the graph (and every usage count read from it) is never left
	// describing the previous extraction.
	stats, err := a.ExtractToProjectStore(ctx, a.FormatReg, layout.Root, pctx, resolved)
	if err != nil {
		return nil, "", err
	}
	return &stats, describeDrift(drift), nil
}

// settleSourceLine renders the settle phase's run-log line. The first count is
// the whole source; the hold is measured over the blocks in files that feed a
// target language, and the line names that subset whenever it is smaller.
func settleSourceLine(s settledSource, level model.TranslateAfterLevel) string {
	settled := len(s.states)
	switch {
	case s.heldOf == 0:
		return fmt.Sprintf("Settled source: %d translatable block(s). No target language reads them, so translate_after %q holds nothing.",
			settled, level)
	case s.heldOf == settled:
		return fmt.Sprintf("Settled source: %d translatable block(s), %d held below translate_after %q.", settled, s.held, level)
	default:
		return fmt.Sprintf("Settled source: %d translatable block(s). Of the %d with a target language, %d held below translate_after %q.",
			settled, s.heldOf, s.held, level)
	}
}

// describeDrift renders a short reason for an auto-extract, for the run log.
// It names why the extraction ran, so it reads beside the counts the
// extraction produced.
func describeDrift(d project.StoreDrift) string {
	switch {
	case d.StoreMissing:
		return "first extraction"
	case d.VersionStale:
		return "store written by another kapi version"
	case len(d.Changed) > 0 && len(d.Removed) > 0:
		return fmt.Sprintf("%d source file(s) changed, %d removed", len(d.Changed), len(d.Removed))
	case len(d.Changed) > 0:
		return fmt.Sprintf("%d source file(s) changed", len(d.Changed))
	default:
		return fmt.Sprintf("%d source file(s) removed", len(d.Removed))
	}
}

// deriveCoverage recomputes per-scope ship coverage from the working tree with
// the same derivation `kapi status` uses, re-read each pass (state is derived,
// never tracked). With withChecks it first runs the project's bound checks over
// the produced units and feeds the failing set into the coverage rollup as an
// exclusion (#1078 G4), returning it so callers can report per-locale counts.
func (a *App) deriveCoverage(ctx context.Context, cmd Command, proj *project.KapiProject, root string, withChecks bool, unread *UnreadSet) ([]LocaleCoverage, *CheckExclusions, error) {
	units, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return nil, nil, err
	}
	var excl *CheckExclusions
	if withChecks {
		excl, err = a.loopCheckExclusions(ctx, cmd, proj, root, units, unread)
		if err != nil {
			return nil, nil, err
		}
	}
	cov, err := a.shipCoverage(ctx, proj, root, units, excl, unread)
	return cov, excl, err
}

// localesNeedingWork is localesNeedingPass with one more kind of work: a
// locale with blocks whose stored draft an earlier kapi wrote (unstamped, by
// locale, from unstampedDrafts). Coverage reads the target files, which may
// hold those translations, so it calls the locale complete; a merge writes
// none of those drafts, and a pass drafts each block again.
func localesNeedingWork(cov []LocaleCoverage, locales []model.LocaleID, unstamped map[string]int) []model.LocaleID {
	pending := localesNeedingPass(cov, locales)
	if len(unstamped) == 0 {
		return pending
	}
	in := make(map[model.LocaleID]bool, len(pending))
	for _, loc := range pending {
		in[loc] = true
	}
	var out []model.LocaleID
	for _, loc := range locales {
		if in[loc] || unstamped[string(loc)] > 0 {
			out = append(out, loc)
		}
	}
	return out
}

// localesNeedingPass returns the locales (in target order) that still have work:
// a scope holding units whose source moved since their translation was decided,
// a gated scope that is not shippable, or (when ungated) content with no
// committed target yet (below the lowest rung).
//
// This is convergence's NEED-based selection. Plain flow runs (the desktop
// runner, RunFromProject, RunFlowAllLocales) share the applicability-based
// flow.ResolveFlowLocales, which asks "which locales does this flow apply to".
// A convergence pass asks "which locales are still short of their gate", so
// converged locales drop out of later passes.
func localesNeedingPass(cov []LocaleCoverage, locales []model.LocaleID) []model.LocaleID {
	var out []model.LocaleID
	for _, loc := range locales {
		l := string(loc)
		needs := false
		for _, c := range cov {
			if c.Locale != l {
				continue
			}
			// A stale unit is WORK, on any scope, gated or not. It tallies at
			// `draft` because it holds a target, so the ungated rung test below
			// would read the scope as complete. The unit needs a translation of
			// the source the project has NOW: recycle against the new wording,
			// AI for the remainder, exactly as an untranslated unit flows.
			if c.Stale > 0 {
				needs = true
				break
			}
			// A rejection is work on the same footing. The unit holds a target,
			// so the ungated rung test below reads the scope as complete, and a
			// reviewer has said the wording will not do. What it needs is
			// another draft, which is production.
			if c.RejectedAwaitingDraft > 0 {
				needs = true
				break
			}
			// A unit failing the project's bound checks is work on any scope,
			// gated or not, for the same reason: it counts at its true rung, so
			// the ungated rung test below reads the scope as complete while the
			// content carries a defect the run reported.
			if c.FailingChecks > 0 {
				needs = true
				break
			}
			if c.Gated && !c.Shippable {
				needs = true
				break
			}
			// Ungated scope: there is work while any unit has no committed target
			// yet (below `draft`, the lowest rung).
			if !c.Gated && c.AtLeast["draft"] < c.Total {
				needs = true
				break
			}
		}
		if needs {
			out = append(out, loc)
		}
	}
	return out
}

// withheldLocales names the locales with a scope that does not ship. Under
// gated delivery such a locale gets none of its files, whatever its other
// scopes clear (finishConverge delivers a locale only when it is shippable).
func withheldLocales(cov []LocaleCoverage) map[string]bool {
	withheld := map[string]bool{}
	for _, c := range cov {
		if !c.Shippable {
			withheld[c.Locale] = true
		}
	}
	return withheld
}

// heldDrafts counts, per locale, the units a run that owns delivery holds for
// review: in each withheld locale, the units its last pass produced (drafted
// with a provider, taken from a stored draft, or recycled from the content
// memory), at most the locale's unit count. A recycled unit the gate holds
// waits for review as a drafted one does. A withheld locale gets no files, so
// these units are written nowhere in the project, and the write tally counts
// none of them.
func heldDrafts(cov []LocaleCoverage, production map[string]convergence.PassProduction) map[string]int {
	units := localeUnits(cov)
	held := map[string]int{}
	for locale := range withheldLocales(cov) {
		p := production[locale]
		if n := min(p.ViaAI+p.ViaDraft+p.ViaMemory, units[locale]); n > 0 {
			held[locale] = n
		}
	}
	return held
}

// localeUnits is each locale's unit count across its scopes.
func localeUnits(cov []LocaleCoverage) map[string]int {
	units := map[string]int{}
	for _, c := range cov {
		units[c.Locale] += c.Total
	}
	return units
}

// sumCounts is the total of a per-locale count.
func sumCounts(m map[string]int) int {
	total := 0
	for _, n := range m {
		total += n
	}
	return total
}

// checkUnitCounts refuses a summary whose counts exceed a language's units.
// Each edition of a language translates that language's units, so for every
// edition the units the run wrote are at most the language's units; for the
// language's own edition, which a gate holds, the units written and the units
// held for review are disjoint subsets of them. A violation is a counting
// defect, and printing it would report work that does not exist.
func checkUnitCounts(cov []LocaleCoverage, produced map[editionAt]int, waiting map[string]int) error {
	units := localeUnits(cov)
	editions := make([]editionAt, 0, len(produced)+len(waiting))
	for e := range produced {
		editions = append(editions, e)
	}
	for l := range waiting {
		if plain := (editionAt{locale: l, edition: l}); produced[plain] == 0 {
			editions = append(editions, plain)
		}
	}
	slices.SortFunc(editions, func(x, y editionAt) int { return strings.Compare(x.edition, y.edition) })
	for _, e := range editions {
		held := 0
		if e.edition == e.locale {
			held = waiting[e.locale]
		}
		if produced[e]+held > units[e.locale] {
			return fmt.Errorf("internal error: %s counts %d produced and %d waiting for review for %d unit(s); the run's summary would overstate its work",
				e.edition, produced[e], held, units[e.locale])
		}
	}
	return nil
}

// finishConverge derives the final per-locale standing, applies the
// materialize policy (#1078 G2), and emits the structured convergence result.
// It always returns nil for parked work (not a build error); only an
// operational materialize failure is an error.
//
// Materialize policy: when the recipe sets `defaults.materialize: on-converge`
// (or the run forces it with --materialize), every locale whose gated scopes
// are ALL shippable has its target files written via the shared
// merge/materialize path, from what the workspace home keeps of it and the
// run's drafts. A parked locale is not delivered, and its drafts are kept in
// the workspace home (host/workhome.go) until it is.
func (a *App) finishConverge(ctx context.Context, cmd Command, proj *project.KapiProject, projectPath, flowName string, passes int, cov []LocaleCoverage, locales []model.LocaleID, translateAfter model.TranslateAfterLevel, blockedOnSource, totalSource int, facts *convergeFacts, opts ConvergeOptions, emit func(convergence.Event)) error {
	out := buildConvergeOutput(flowName, passes, cov, locales, facts.redraftable)
	out.Warnings = facts.unread.warnings()
	// A source whose name cannot fill its target template got no file; the
	// run passed over it and names it here.
	out.Warnings = append(out.Warnings, a.unsafeTargetWarnings(proj, filepath.Dir(projectPath), "")...)
	out.Monolingual = facts.monolingual
	out.NoTargets = facts.noTargets
	out.ExtractedFiles = facts.extractedFiles
	out.ExtractedBlocks = facts.extractedBlocks
	out.WaitingForReview = facts.waitingForReview
	out.BlockedOnSource = blockedOnSource
	out.Findings = facts.placeholderReport()
	if translateAfter != "" {
		out.TranslateAfter = string(translateAfter)
	}
	// Hold-on-source is a first-class outcome (epic 019). When EVERY translatable
	// source block is held (nothing is producible), the run held on SOURCE. It
	// is labelled source_not_ready so the summary points at settling the source
	// rather than a review queue, and marked un-converged even when no ship gate
	// applies. This mirrors the server's gateItemsBySource: producible==0 &&
	// held>0 → source_not_ready. A PARTIAL run (some source ready, some held)
	// translates the ready blocks and reports the held count, and is not
	// source_not_ready, as the server distinguishes a full hold from a partial
	// one.
	if blockedOnSource > 0 && blockedOnSource >= totalSource {
		out.StallReason = convergence.StallSourceNotReady
		out.Converged = false
		// Nothing was producible, so nothing was re-drafted. The units are still
		// stale and still reported; what they wait on is the source, which the
		// hold line above already names.
		for i := range out.Locales {
			out.Locales[i].Redrafted = 0
		}
	}

	// A source-held run produced no translations (epic 019): do NOT materialize.
	// Every locale's source-fallback output would otherwise be written as if it
	// were caught up, which translate_after exists to prevent. The server skips
	// its post-run work the same way when a run stalls on source_not_ready.
	//
	// Delivery commits each draft through the file home and records what it
	// changed, with the basis of each translation it wrote (host/flowdrafts.go).
	// A run that prints its change set prints what delivery would commit, for
	// the locales that clear their gate, and stamps nothing.
	gated := deliveryIsGated(proj, opts)
	if gated && out.StallReason == convergence.StallSourceNotReady {
		a.printOps.note("the source is not ready to translate, so kapi up delivers no file and none is printed")
	}

	producedBeforeDelivery := a.writeTally.total()
	if gated && out.StallReason != convergence.StallSourceNotReady {
		for i := range out.Locales {
			lc := &out.Locales[i]
			if !lc.Shippable {
				// Parked and pending locales are not delivered. Under this
				// policy that is the whole of delivery (the pass wrote into the
				// run's draft tree), so the locale has no files at all. The drafts are kept in the
				// workspace home, where a reviewer reads and edits them and the
				// next delivery finds them.
				a.printOps.note(lc.Locale + ": short of its ship gate, so kapi up delivers none of its files and none is printed")
				if a.printOps == nil {
					if _, kerr := a.keepDrafts(ctx, filepath.Dir(projectPath), model.LocaleID(lc.Locale), false); kerr != nil {
						return fmt.Errorf("keep the %s drafts: %w", lc.Locale, kerr)
					}
				}
				continue
			}
			// The locale cleared its gate, so its drafts become its delivery.
			// This moves the run's own output, which is the record that exists
			// for every flow; the store-backed write below adds what the
			// workspace home keeps of the locale and whatever overlays the
			// run also committed. The run's drafts replace the drafts the
			// workspace kept from an earlier run first, so what is delivered
			// on top is the edits made to them, never a draft of a source that
			// has changed since.
			if a.printOps == nil {
				if _, kerr := a.keepDrafts(ctx, filepath.Dir(projectPath), model.LocaleID(lc.Locale), true); kerr != nil {
					return fmt.Errorf("keep the %s drafts: %w", lc.Locale, kerr)
				}
			}
			delivered, derr := a.deliverDrafts(ctx, model.LocaleID(lc.Locale))
			if derr != nil {
				return fmt.Errorf("deliver %s: %w", lc.Locale, derr)
			}
			if a.printOps != nil {
				continue
			}
			// Per-file progress lines go nowhere: the structured result carries
			// the counts, and stray lines would corrupt --json output. A stored
			// draft of a source that changed since, or one an earlier kapi
			// wrote, is not written (materializeEdition), and the result counts
			// it: coverage counts an overlay by its presence and never reads
			// the stamp.
			n, staleDrafts, merr := a.materializeProject(ctx, io.Discard, proj, projectPath, []model.LocaleID{model.LocaleID(lc.Locale)}, false, facts.unread)
			if merr != nil {
				return fmt.Errorf("materialize %s: %w", lc.Locale, merr)
			}
			if n < len(delivered) {
				n = len(delivered)
			}
			lc.Materialized = n
			lc.StaleDrafts = staleDrafts.Blocks
			lc.UnstampedDrafts = staleDrafts.Unstamped
			out.MaterializedFiles += n
		}
		if out.MaterializedFiles > 0 {
			// The delivery is where a gated run writes its target files, so
			// the units it wrote are reported here.
			emit(convergence.Event{Type: convergence.EventMaterialized, Files: out.MaterializedFiles,
				ProducedDelta: a.writeTally.total() - producedBeforeDelivery})
		}

		// Delivery is done, so the drafts stop being anybody's subject and the
		// standing is re-read off the tree the run leaves behind.
		//
		// Delivery is graded against the drafts. What the run REPORTS is what a
		// reader will find: for a parked locale, the file already on disk. So
		// `kapi up` and `kapi status` agree over one locale (#2024).
		//
		// The verdict does not move by re-reading. A locale is pending, and so
		// parked, because of staleness, a failing check, or a gate it has not
		// cleared, and each of those holds on the delivered file too.
		a.endConvergeDrafts()
		cov, _, rerr := a.deriveCoverage(ctx, cmd, proj, filepath.Dir(projectPath), !opts.noChecks, facts.unread)
		if rerr != nil {
			return fmt.Errorf("re-derive the delivered standing: %w", rerr)
		}
		report := buildConvergeOutput(flowName, passes, cov, locales, facts.redraftable)
		// How many files each locale got, and how many stale drafts its
		// materialize left out, are the run's own facts, not the tree's.
		materialized := make(map[string]ConvergeLocaleResult, len(out.Locales))
		for _, lc := range out.Locales {
			materialized[lc.Locale] = lc
		}
		for i := range report.Locales {
			ran := materialized[report.Locales[i].Locale]
			report.Locales[i].Materialized = ran.Materialized
			report.Locales[i].StaleDrafts = ran.StaleDrafts
			report.Locales[i].UnstampedDrafts = ran.UnstampedDrafts
		}
		out.Locales = report.Locales
		out.ParkedScopes = report.ParkedScopes
		out.Converged = report.Converged
	}

	// Everything this run wrote is the store's own output. Stamping it as
	// absorbed is what keeps the next run from reading its own materialization
	// back as a statement from git; see stampCommittedRecord.
	if a.printOps == nil {
		a.stampCommittedRecord(ctx, proj, projectPath, facts.unread)
		// A translation whose file now exists, whichever path wrote it, has
		// its file as its home: the workspace home settles what it kept.
		if rerr := a.settleKept(ctx, projectPath); rerr != nil {
			return fmt.Errorf("settle the drafts the workspace home keeps: %w", rerr)
		}
	}

	// Produced is what the run wrote to target files, by a pass or by the
	// delivery above: each translation the change service recorded with
	// content its file did not hold.
	produced := a.writeTally.total()
	out.Produced = &produced
	if err := checkUnitCounts(cov, a.writeTally.byEdition(), facts.held); err != nil {
		return err
	}

	state := convergence.RunConverged
	if !out.Converged {
		state = convergence.RunParked
	}
	emit(convergence.Event{
		Type:            convergence.EventDone,
		State:           state,
		StallReason:     out.StallReason,
		BlockedOnSource: out.BlockedOnSource,
	})

	if opts.capture != nil {
		*opts.capture = out
		return nil
	}
	return output.Print(cmd, out)
}

// buildConvergeOutput rolls the per-scope coverage up into the per-locale
// convergence result.
// The per-locale figures are read out of the coverage rows and nowhere else:
// the percentages, the failing-check counts and the verdict all come from the
// same rollup `kapi status` and `kapi status --ship` read, so the three surfaces
// cannot disagree about one locale (#2024).
func buildConvergeOutput(flowName string, passes int, cov []LocaleCoverage, locales []model.LocaleID, redraftable map[string]int) ConvergeOutput {
	out := ConvergeOutput{Flow: flowName, Passes: passes, Converged: true}
	pendingSet := map[string]bool{}
	for _, loc := range localesNeedingPass(cov, locales) {
		pendingSet[string(loc)] = true
	}
	for _, loc := range locales {
		l := string(loc)
		res := ConvergeLocaleResult{Locale: l, Shippable: true, ShipState: ShipStateEstablished,
			Pct: map[string]int{}, Redrafted: redraftable[l]}
		gatedSomewhere := false
		scoped := false
		total := 0
		atLeast := map[string]int{}
		for _, c := range cov {
			if c.Locale != l {
				continue
			}
			scoped = true
			total += c.Total
			for k, v := range c.AtLeast {
				atLeast[k] += v
			}
			res.Stale += c.Stale
			res.Rejected += c.RejectedAwaitingDraft
			res.FailingChecks += c.FailingChecks
			// The verdict is the rollup's, unconditionally: a scope that does not
			// ship does not ship, gate or no gate, and reading it only for gated
			// scopes is how a locale held back by stale or failing units still
			// reported itself shippable.
			if !c.Shippable {
				res.Shippable = false
			}
			if c.Gated {
				gatedSomewhere = true
			}
			res.ShipState = weakerShipState(res.ShipState, c.ShipState)
		}
		// The locale's percentages are over its scopes' units together.
		for k, v := range atLeast {
			res.Pct[k] = convergence.Percent(v, total)
		}
		res.Gated = gatedSomewhere
		// A locale with no coverage row at all has nothing no gate matched.
		if !scoped {
			res.ShipState = ShipStateNotGated
		}
		if !res.Shippable {
			out.Converged = false
			// Parked is "the loop left work here": short of a gate, or held by a
			// fact (stale wording, a failing guardrail) the loop cannot settle on
			// its own. An ungated locale with neither is not gated.
			res.Parked = pendingSet[l] && (gatedSomewhere || res.Stale > 0 || res.FailingChecks > 0)
		}
		out.Locales = append(out.Locales, res)
	}
	// Per-scope parked detail: every (collection, locale) scope that does not
	// ship, in the coverage's stable order: short of its gate, or holding stale
	// pairings or failing checks a review surface should be pointed at.
	for _, c := range cov {
		if !c.Shippable && (c.Gated || c.Stale > 0 || c.FailingChecks > 0) {
			out.ParkedScopes = append(out.ParkedScopes, ParkedScope{Locale: c.Locale, Collection: c.Collection})
		}
	}
	return out
}

// ConvergeMaxPassesDefault caps the --until-gate loop. A handful of passes is
// plenty: a deterministic flow converges in one, and a stalled unit parks rather
// than spinning.
const ConvergeMaxPassesDefault = 5

// builtinDefaultFlowName is the name the built-in default convergence flow
// runs under when a recipe sets no defaults.flow (#1078 G6).
const builtinDefaultFlowName = "default"

// BuiltinDefaultFlowLabel is how the built-in default flow is reported in
// structured output (ConvergeOutput.Flow / UpPlanOutput.Flow), so a reader can
// tell it apart from a recipe-defined flow that happens to be named "default".
const BuiltinDefaultFlowLabel = "default (built-in)"

// convergeFlow is the flow `kapi up` runs for a project: the steps as the pass
// executes them, the name a run reports them under, and the name the flow is
// registered as.
type convergeFlow struct {
	name  string
	label string
	spec  *flow.StepsSpec
	// translateAfter is the level the leading translate-after stage holds
	// blocks below, or TranslateAfterNone when the recipe opts out and no stage
	// is prepended.
	translateAfter model.TranslateAfterLevel
	// notes names each run flag a step keeps its own value over.
	notes []string
}

// convergeFlowSpec resolves the flow a convergence pass executes: the recipe's
// defaults.flow, or the built-in default when the recipe names none, with the
// translate-after leading stage prepended unless the level is `none`. The run and
// `kapi up --plan` both resolve through it. A plan prices only a flow the run
// would execute, and a flow the run cannot start fails the plan the same way. projectDir is the recipe's directory, which a `flows_dir:` is relative
// to. runConfig is the run flags' tool config (runFlagToolConfig), nil when no
// command is in play. It seeds every step of the flow, the built-in default
// and a recipe flow alike, by the rule seedRunConfig states, as `kapi run`
// seeds the flow it runs.
func convergeFlowSpec(proj *project.KapiProject, projectDir string, runConfig map[string]any) (convergeFlow, error) {
	cf := convergeFlow{name: proj.Defaults.Flow, label: proj.Defaults.Flow}
	if cf.name == "" {
		// No defaults.flow: run the built-in default (#1078 G6), content memory
		// reuse then AI translate, so `kapi up` works with zero flow YAML. An
		// explicitly configured defaults.flow always wins over this synthesis.
		cf.name = builtinDefaultFlowName
		cf.label = BuiltinDefaultFlowLabel
		cf.spec, cf.notes = seedRunConfigSpec(DefaultConvergeFlowSpec(), runConfig)
	} else {
		// The name resolves as `kapi run` resolves it (ResolveProjectFlow).
		pf, err := ResolveProjectFlow(proj, projectDir, cf.name, runConfig)
		if err != nil {
			return cf, err
		}
		if pf == nil {
			return cf, fmt.Errorf("defaults.flow %q names no flow under the recipe's `flows:`, no file in its `flows_dir:` and no built-in flow", cf.name)
		}
		cf.spec, cf.notes = pf.Spec, pf.Notes
	}

	// Source-first convergence: resolve defaults.translate_after. When it is
	// not `none`, a leading translate-after stage settles each block's source
	// authoring status and holds a block whose source is below the level, the
	// local, in-stream counterpart of the server's settleSource +
	// gateItemsBySource. The stage prepends to the flow so it runs before
	// recycle/translate, which skip a held block. There is no stage and no hold
	// when the project opts out with `translate_after: none`, matching the
	// server's "raw MT" path.
	cf.translateAfter, _ = convergeTranslateAfter(proj)
	if cf.translateAfter != model.TranslateAfterNone {
		cf.spec = prependTranslateAfter(cf.spec, cf.translateAfter)
	}
	return cf, nil
}

// DefaultConvergeFlowSpec returns the built-in default convergence flow used
// when a recipe configures no defaults.flow: content memory reuse (recycle) then
// AI translate over the REMAINDER. It needs no qa step: the convergence loop
// already runs the project's bound checks after each pass (#1078 G4). A fresh
// spec is returned per call so callers can never mutate a shared instance.
//
// skip_matched makes the second step mean "the remainder", and the pass needs
// it. The translate tool's own default is to translate every block it is
// handed, which suits `kapi exec translate`. In a convergence pass the
// recycled targets hold the project's own reviewed wording, and skip_matched
// keeps the model from overwriting them and the approval bound to each. The
// built-in `translate` flow (host/flowdef) sets it for the same reason.
func DefaultConvergeFlowSpec() *flow.StepsSpec {
	return &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "recycle"},
		{Tool: "translate", Config: map[string]any{"skip_matched": true}},
	}}
}

// recycleOnlyFlowName labels a recycle-only pass in its events and run record.
const recycleOnlyFlowName = "recycle"

// RecycleOnlyFlowSpec returns the flow of a recycle-only pass: content memory
// reuse with exact matches alone. Both thresholds are 100, so a near match
// (the 95 to 99 band the default flow fills) stays for a full run, where a
// person asked for the work. An ambiguous exact match, several full-score
// entries with different targets, is never filled (core/tools.leverageBlock).
func RecycleOnlyFlowSpec() *flow.StepsSpec {
	return &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "recycle", Config: map[string]any{
			"fuzzy_threshold":       100,
			"fill_target_threshold": 100,
		}},
	}}
}

// prependTranslateAfter returns a copy of spec with the translate-after
// leading stage inserted at the head: the leading source-transform stage that
// settles the source and holds blocks below the level before any producer
// runs. The level rides in the step config so each per-locale worker builds a
// translate-after instance for the resolved level. The input spec is not
// mutated (a fresh spec + steps slice is returned) so a project's shared flow
// definition is never rewritten. A spec that already opens with a
// translate-after step (a recipe that placed it explicitly) is left as-is, so
// the stage is not doubled.
func prependTranslateAfter(spec *flow.StepsSpec, level model.TranslateAfterLevel) *flow.StepsSpec {
	if spec == nil {
		spec = &flow.StepsSpec{}
	}
	if len(spec.Steps) > 0 && spec.Steps[0].Tool == "translate-after" {
		return spec
	}
	steps := make([]flow.FlowStep, 0, len(spec.Steps)+1)
	steps = append(steps, flow.FlowStep{
		Tool:   "translate-after",
		Config: map[string]any{"level": string(level)},
	})
	steps = append(steps, spec.Steps...)
	out := *spec
	out.Steps = steps
	return &out
}
