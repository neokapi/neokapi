package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
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
	// onEvent, when set, receives the run's convergence.Event stream — the
	// one protocol every surface renders a run from (CLI live view, NDJSON,
	// desktop run view, server SSE). Called from one goroutine at a time.
	onEvent func(convergence.Event)
	// capture, when non-nil, receives the final ConvergeOutput instead of it
	// being printed through the command's output formatter (embedders).
	capture *ConvergeOutput
}

// ConvergeLocaleResult is the per-locale outcome of a convergence run.
type ConvergeLocaleResult struct {
	Locale    string         `json:"locale"`
	Shippable bool           `json:"shippable"`        // every scope for this locale clears its ship gate
	Verified  bool           `json:"verified"`         // every scope for this locale clears its verified gate
	Parked    bool           `json:"parked,omitempty"` // the loop left work here (needs human)
	Pct       map[string]int `json:"pct,omitempty"`    // ladder state → "at least" percent
	// FailingChecks counts units that are produced but fail the project's bound
	// checks (#1078 G4). They count at their true rung in Pct — the unit is
	// translated — and hold the locale out of Shippable until fixed. It counts
	// UNITS, which is not what `kapi check` counts: one unit can carry several
	// findings, and `kapi check` lists findings.
	FailingChecks int `json:"failingChecks,omitempty"`
	// Stale counts units whose translation was decided against source wording
	// that has since changed. They hold the locale out of Shippable whether or
	// not a gate applies: the decision on record no longer describes the
	// project, and only a person can replace it.
	Stale int `json:"stale,omitempty"`
	// Redrafted counts the stale units this run produced over — recycled against
	// the rewritten source, or drafted for the remainder. It is what the loop
	// owes them: a translation of the source the project has now. The decision
	// they carried is NOT restored, so they stay in Stale until someone reviews
	// the new draft, which is why the two are reported side by side.
	Redrafted int `json:"redrafted,omitempty"`
	// Materialized counts the target files written for this locale by the
	// post-loop materialize step (defaults.materialize: on-converge, or
	// --materialize). Only shippable locales materialize; a parked locale
	// stays at 0.
	Materialized int `json:"materialized,omitempty"`
}

// ParkedScope identifies one gated (collection, locale) scope still short of
// its gate after the run — the address a review surface can deep-link to.
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
	Converged bool                   `json:"converged"` // every gated scope is shippable
	Locales   []ConvergeLocaleResult `json:"locales"`
	// ParkedScopes lists the gated (collection, locale) scopes that remain
	// short of their gate — per-scope detail under the per-locale rollup, so
	// a UI can link each parked scope to its review queue.
	ParkedScopes []ParkedScope `json:"parkedScopes,omitempty"`
	// MaterializedFiles is the total count of target files written by the
	// post-loop materialize step across every shippable locale (0 when the
	// policy is manual and --materialize was not passed).
	MaterializedFiles int `json:"materializedFiles,omitempty"`
	// BlockedOnSource is how many translatable source blocks were held below the
	// active source gate this run (epic 019): their translations were NOT
	// produced because the source is un-settled. 0 when the gate is `none` or the
	// source is fully settled. It is source-scoped (deduped across locales — a
	// source block is shared by every target), mirroring the server's run row.
	BlockedOnSource int `json:"blockedOnSource,omitempty"`
	// SourceGate is the resolved source-first gate level applied
	// (none|authored|checked|approved), for observability. Empty when no gate
	// was evaluated (no content).
	SourceGate string `json:"sourceGate,omitempty"`
	// StallReason is the machine-readable cause a run did not converge — set to
	// source_not_ready when every pending locale had nothing producible because
	// its source is held below the gate. Empty on a clean/parked-on-target run.
	StallReason string `json:"stallReason,omitempty"`
	// Monolingual reports a run over a project that resolves no target locale at
	// all: the source half of convergence ran and the per-locale fan-out was not
	// applicable. Locales is empty for such a run, and a reader needs to tell
	// that apart from a multilingual project whose locales all dropped out.
	Monolingual bool `json:"monolingual,omitempty"`
	// ExtractedFiles and ExtractedBlocks report the pre-pass re-extraction that
	// brought the project store back in sync with the working tree. Both 0 when
	// the store was already current. They are the "what moved" of a monolingual
	// run, whose whole output is otherwise source-side.
	ExtractedFiles  int `json:"extractedFiles,omitempty"`
	ExtractedBlocks int `json:"extractedBlocks,omitempty"`
	// ConceptsProposed, ChangesetID and ChangesetURL report governed terminology
	// the run proposed rather than applied: edits that take effect only once a
	// reviewer accepts them. They are venue-supplied — a run at a venue that
	// governs terminology fills them in — and all three are zero for a run that
	// proposed nothing.
	//
	// ChangesetURL is a deep link to the review surface, and is empty whenever
	// the venue could not name one. An id with no link is still worth reporting:
	// it is the handle every other surface takes, so a reader who cannot click
	// through can still ask for the change-set by name.
	ConceptsProposed int    `json:"conceptsProposed,omitempty"`
	ChangesetID      string `json:"changesetId,omitempty"`
	ChangesetURL     string `json:"changesetUrl,omitempty"`
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
	if o.Monolingual {
		return o.formatMonolingual(w)
	}
	verb := "pass"
	if o.Passes != 1 {
		verb = "passes"
	}
	fmt.Fprintf(w, "Ran flow %q over %d locale(s) in %d %s.\n\n", o.Flow, len(o.Locales), o.Passes, verb)
	t := output.NewTable(w).Accent(0).
		Headers("locale", "drafted", "translated", "state", "checks")
	s := t.Styles()
	for _, lc := range o.Locales {
		state := s.Warn.Render("pending")
		switch {
		case lc.Parked:
			state = s.Warn.Render("parked (needs human)")
		case lc.Shippable:
			state = s.Success.Render("✓ shippable")
		}
		// Units, named as units. `kapi check` counts finding rows over the same
		// tree and one unit can carry several, so two numbers that were both
		// labelled "check(s)" read as a contradiction between the commands.
		checks := s.Dim("")
		if lc.FailingChecks > 0 {
			checks = s.Error.Render(fmt.Sprintf("%d unit(s) failing checks", lc.FailingChecks))
		}
		t.Row(lc.Locale,
			fmt.Sprintf("%d%%", lc.Pct["draft"]),
			fmt.Sprintf("%d%%", lc.Pct["translated"]),
			state, checks)
	}
	t.Render()
	fmt.Fprintln(w)
	// Source hold (epic 019): when the source-first gate held blocks below the
	// bar, say so plainly — those blocks were NOT translated, and the fix is to
	// settle the source, not to wait for review. Never a silent skip.
	if o.BlockedOnSource > 0 {
		fmt.Fprintf(w, "%d block(s) held on source — settle your source first "+
			"(kapi check --ship, or fix terms/brand/source and kapi apply), then re-run.\n", o.BlockedOnSource)
	}
	// Source drift under a decided translation, and the two halves of settling
	// it. The loop owns the first: a translation of the source the project has
	// now. It cannot own the second — the decision that was withdrawn was a
	// person's — so the two are reported together rather than either standing
	// for the whole. Drift that reports itself as converged is what these lines
	// exist to make impossible; drift the loop declines to work on is what the
	// re-draft line exists to make impossible.
	if redrafted := o.RedraftedUnits(); redrafted > 0 {
		fmt.Fprintf(w, "Re-drafted %d stale unit(s) against the current source — "+
			"they return to review un-approved (the earlier approval is not restored).\n", redrafted)
	}
	if stale := o.StaleUnits(); stale > 0 {
		fmt.Fprintf(w, "%d unit(s) stale — their source changed since the translation was decided. "+
			"Re-review them (kapi status --review); they do not ship until you do.\n", stale)
	}
	if o.Converged {
		fmt.Fprintln(w, "Up to date: every gated scope is shippable.")
	} else if o.StallReason == convergence.StallSourceNotReady {
		fmt.Fprintln(w, "Held on source — nothing translatable is settled yet. Set defaults.source_gate: none to draft freely.")
	} else {
		fmt.Fprintln(w, "Not yet up to date — parked locales await human review (never a build failure).")
	}
	if o.MaterializedFiles > 0 {
		fmt.Fprintf(w, "Materialized %d target file(s) from the project store.\n", o.MaterializedFiles)
	}
	o.formatProposedChangeset(w)
	return nil
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
	fmt.Fprintf(w, "Proposed %d governed terminology edit(s) in change-set %s — they take effect when reviewed.\n",
		o.ConceptsProposed, o.ChangesetID)
	if o.ChangesetURL != "" {
		fmt.Fprintf(w, "Review it at %s\n", o.ChangesetURL)
	}
}

// formatMonolingual renders a run over a project with no target locale. The
// locale grid is not printed empty: a table with no rows reads as "nothing was
// reconciled", when in fact the whole source half of the run — seed, extract,
// occurrence graph — completed. What moved is the store, so that is what the
// summary reports.
func (o ConvergeOutput) formatMonolingual(w io.Writer) error {
	fmt.Fprintf(w, "Reconciled the source — no target languages configured, so the per-language flow %q did not run.\n\n", o.Flow)
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
// over its content across every target language — the no-argument `kapi run`.
// Without untilGate it runs a single pass; with untilGate it loops the pass,
// re-deriving coverage each time, until every gated scope is shippable, a pass
// makes no progress, or maxPasses is reached — parking the locales that remain
// short of their gate. It never fails the build: parked work is reported, not an
// error (target drift is normal toil, not a break).
func (a *App) RunDefaultFlowConverge(cmd Command, proj *project.KapiProject, projectPath string, opts ConvergeOptions) error {
	untilGate, maxPasses := opts.UntilGate, opts.MaxPasses
	flowName := proj.Defaults.Flow
	flowLabel := flowName
	var spec *flow.StepsSpec
	if flowName == "" {
		// No defaults.flow: run the built-in default (#1078 G6) — content memory reuse then
		// AI translate — so `kapi up` works with zero flow YAML. An explicitly
		// configured defaults.flow always wins over this synthesis.
		flowName = builtinDefaultFlowName
		flowLabel = BuiltinDefaultFlowLabel
		spec = DefaultConvergeFlowSpec()
	} else {
		spec = proj.Flow(flowName)
		if spec == nil {
			if BuiltinComposedFlowNames()[flowName] {
				return fmt.Errorf("defaults.flow %q is a built-in flow; define it under the project's `flows:` map to use it as `kapi up`'s default flow", flowName)
			}
			return fmt.Errorf("default flow %q not found in the project's `flows:`", flowName)
		}
	}

	// Source-first gate (epic 019): resolve the convergence source gate
	// (defaults.source_gate). When it is active (not `none`), a leading
	// source-gate stage settles each block's source authoring status and holds a
	// block whose source ranks below the gate — the local, in-stream counterpart
	// of the server's settleSource + gateItemsBySource. The stage prepends to the
	// flow so it runs before recycle/translate, which skip a held block. The gate
	// is off (no stage, no hold) when the project opts out with `source_gate:
	// none` — parity with the server "raw MT, no gate" path.
	sourceGate, _ := convergeSourceGate(proj)
	if sourceGate != model.SourceGateNone {
		spec = prependSourceGate(spec, sourceGate)
	}

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
	var sources []string
	for _, rf := range resolved {
		sources = append(sources, rf.Path)
	}
	if len(sources) == 0 {
		return errors.New("no content to catch up (add content patterns to the project)")
	}

	// The locales this run fans out over, and the verdict that decides whether it
	// fans out at all. A project that resolves none is MONOLINGUAL — the front
	// door, not a misconfiguration: the source half of convergence still runs
	// (the committed context seeds, the working tree re-extracts into the store,
	// the occurrence graph refreshes) and only recycle/translate/materialize are
	// not applicable. Not-applicable is not an error, and refusing to start left
	// a one-language project with no way to compile its own sources into the
	// store at all.
	locales := contentTargetLocales(proj.Defaults, resolved)
	monolingual := len(locales) == 0

	// Self-seeding (AD-009/AD-010 custody): the committed context sources are the
	// truth and the store is their projection, so they compile on the way in —
	// before anything resolves a binding out of the store. Without this the store
	// only ever holds what a previous write left in it: nothing at all on a fresh
	// clone, and the pre-pull state after a `git pull` of an edited bundle. The
	// compile is digest-keyed, so an unchanged source costs a read and no writes.
	//
	// A seeding failure is PROPAGATED. A run that silently proceeds on an empty
	// store reports full coverage over a corpus it never had the terminology or
	// the reviewed wording for, and spends a provider re-translating what git
	// already carries.
	if seeded, serr := a.SeedProjectContext(ctx, projectPath); serr != nil {
		return fmt.Errorf("seed committed context: %w", serr)
	} else if seeded.Compiled() && !a.Quiet {
		fmt.Fprintln(cmd.ErrOrStderr(), formatSeedLine(seeded))
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
	// because the term rules in them are per target locale: the wording a
	// concept is approved for in nb is not the one it is approved for in fr, and
	// a set resolved here, before any locale is known, would carry neither.
	groupBindings := a.newLocaleBindings(cmd, proj, projectPath)

	root := filepath.Dir(projectPath)
	absProjectPath, _ := filepath.Abs(projectPath)
	projectDir := filepath.Dir(absProjectPath)

	savedTarget := a.TargetLang
	defer func() { a.TargetLang = savedTarget }()

	// Convergence writes real localized files (not just block-store overlays) so
	// its file-derived coverage sees each pass's output — uniformly across
	// single- and multi-file projects.
	a.convergeWriteFiles = true
	defer func() { a.convergeWriteFiles = false }()

	// Under a policy that gates delivery, those files are DRAFTS: the pass
	// writes them into the run's own tree and the policy-gated materialize in
	// finishConverge is the only write that reaches a collection's `target:`
	// path. Otherwise the promise `on-converge` makes — a locale short of its
	// ship gate does not get its files — holds for a second, redundant write
	// while the unreviewed draft is already sitting where a site build globs
	// (#1936). See host/convergedrafts.go.
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
	facts := &convergeFacts{monolingual: monolingual}

	// The venue-neutral loop (core/convergence.Loop) owns the semantics —
	// pass barrier, per-locale fan-out, stall-parks-the-rest; these closures
	// are the CLI venue's IO: working-tree drift re-extract, file-derived
	// coverage with bound checks, and the default flow on per-locale worker
	// Apps.
	derive := func(cov []LocaleCoverage, excl *CheckExclusions) convergence.PassState {
		pending := localesNeedingPass(cov, locales)
		facts.noteRedraftable(cov, pending)
		return convergence.PassState{
			Pending:       localeStrings(pending),
			Produced:      producedUnits(cov),
			FailingChecks: excl.totalFailing(),
			UnitTotals:    localeUnitTotals(cov),
			Detail:        derivedState{cov: cov, excl: excl},
		}
	}
	funcs := convergence.LoopFuncs{
		Derive: func(ctx context.Context) (convergence.PassState, error) {
			cov, excl, err := a.deriveCoverage(ctx, cmd, proj, root, !opts.noChecks)
			if err != nil {
				return convergence.PassState{}, err
			}
			return derive(cov, excl), nil
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
				if err = worker.runProjectStepsOver(ctx, cmd, flowName, spec, &rCtx, group.Inputs); err != nil {
					break
				}
			}
			stopWatch()
			if err != nil {
				return convergence.PassProduction{}, err
			}
			facts.notePassed(locale)
			return tap.snapshot(), nil
		},
	}
	if !opts.noExtract {
		// Auto-extract on drift (#1078 C2): before each pass, bring the
		// project block store back in sync with the working tree — a missing
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
			// they never fail the pass — but left unsaid they make kapi
			// re-extract and re-translate everything on every run with no
			// visible cause. Independent of the event consumer (this is an
			// operator fault, not run progress) but respecting --quiet, as
			// every other warning here does; a caller reading ExtractStats
			// still gets them in Warnings either way.
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
	// (#1449). A blocked path — most often a directory standing where the writer
	// needs a file — is an operational fault, not target-language drift, and a
	// run that cannot write its outputs must not start: the locale would either
	// fail mid-pass (throwing away the other locales' in-flight work and the AI
	// spend behind it) or, before the measurement fix, be counted as fully
	// translated by file-presence and reported "up to date".
	//
	// The resolve error is PROPAGATED, not skipped: swallowing it is the very
	// shape of defect this change closes, and it costs nothing to be strict —
	// UnitsFromProject is a pure re-resolution of the recipe's content patterns,
	// and the loop's own Derive (deriveCoverage) calls it identically and hard-
	// fails a moment later. Failing here just names it sooner.
	admissionUnits, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return fmt.Errorf("resolve project content: %w", err)
	}
	if berr := CheckTargetPathsWritable(admissionUnits); berr != nil {
		return berr
	}

	// Share one parse cache across every pass: unchanged source files parse once,
	// not once per pass; only the targets a pass rewrites re-parse.
	return a.withParseCache(root, func() error {
		// Source-first settle phase (epic 019): settle the source ONCE before the
		// loop (as the server's runSettleSource does), stamp SourceStatus over the
		// distinct source files, and count how many blocks are held below the
		// gate — the number surfaced as "N blocks held on source". A no-op when the
		// gate is `none`.
		//
		// The settle error is PROPAGATED. It used to be swallowed (`herr == nil`)
		// on the stated grounds that the run "degrades to the gate-off behavior",
		// but it does not degrade — it MISREPORTS: with the error dropped,
		// blockedOnSource and totalSource both stay 0, so finishConverge's
		// `blockedOnSource >= totalSource` test is false, the run is labelled
		// converged rather than source_not_ready, and the materialize guard that
		// exists to stop source-fallback output being written "as if caught up" is
		// disarmed. Nor was it ever a survivable degradation: settleAndCountHeldSource
		// fails only on a source-file read, and ComputeShipCoverage reads the same
		// source files through the same readBlocks and hard-fails on them a moment
		// later inside the loop. Strictness costs nothing and names the fault sooner.
		//
		// admissionUnits is reused rather than re-resolved: it is the identical
		// pure re-resolution of the recipe's content patterns, already resolved
		// (and hard-failed on) above.
		var blockedOnSource, totalSource int
		if sourceGate != model.SourceGateNone {
			held, total, unreadable, herr := a.settleAndCountHeldSource(ctx, root, a.SourceLang, sourceGate, admissionUnits)
			if herr != nil {
				return fmt.Errorf("settle source for the %q gate: %w", sourceGate, herr)
			}
			blockedOnSource, totalSource = held, total
			// A collection whose format has no reader on this machine was never
			// opened, so it contributed nothing to the two counts above. That is
			// survivable (the plugin supplying it is optional), but it is not
			// silent: a gate that reports "0 held" over content it could not read
			// is indistinguishable from one that read it and found it clean.
			for _, f := range unreadable {
				emitter.Emit(convergence.Event{
					Type:  convergence.EventLog,
					Stage: convergence.StageSettleSource,
					Message: fmt.Sprintf("No reader for format %q — its content is not counted in the source gate. "+
						"Install the plugin that supplies it (kapi plugins install %s).", f, f),
				})
			}
			emitter.Emit(convergence.Event{
				Type:            convergence.EventLog,
				Stage:           convergence.StageSettleSource,
				BlockedOnSource: held,
				Message: fmt.Sprintf("Settled source: %d block(s) checked, %d held below the %q gate.",
					total, held, sourceGate),
			})
		}

		res, err := convergence.Loop(ctx, convergence.LoopOptions{
			UntilGate: untilGate,
			MaxPasses: maxPasses,
			Jobs:      jobs,
		}, funcs, emitter)
		if err != nil {
			return err
		}
		d := res.Final.Detail.(derivedState)
		return a.finishConverge(ctx, cmd, proj, projectPath, flowLabel, res.Passes, d.cov, locales,
			sourceGate, blockedOnSource, totalSource, facts, opts, emitter.Emit)
	})
}

// convergeFacts are the run-scoped facts finishConverge reports that the
// coverage rollup cannot derive: whether the project fanned out at all, what the
// pre-pass extraction moved, and the work the run took in.
type convergeFacts struct {
	monolingual     bool
	extractedFiles  int
	extractedBlocks int
	// redraftable is, per locale, how many stale units the run STARTED with in
	// scopes it then fanned out over. The final rollup cannot recover it: a
	// re-drafted unit is still stale (its basis names source that is gone, and
	// producing a translation does not decide it), so the count that says "the
	// loop did its half" has to be taken before the half is done.
	redraftable map[string]int
	// passed is the locales a pass actually ran for. Together with the delivery
	// policy it decides which target files hold this run's own output, which is
	// what entitles the run to record a basis for the units inside them
	// (host/basisrecord.go): a file the run did not write holds somebody else's
	// translation of a source the run cannot name.
	//
	// Guarded because the loop fans its passes out across locales in parallel.
	mu     sync.Mutex
	passed map[string]bool
}

// notePassed records that a pass ran for this locale.
func (f *convergeFacts) notePassed(locale string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.passed == nil {
		f.passed = map[string]bool{}
	}
	f.passed[locale] = true
}

// ranPass reports whether any pass ran for this locale.
func (f *convergeFacts) ranPass(locale string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.passed[locale]
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
// It resolves exactly as UnitsFromProject does — same per-item fallback, same
// skip of an item with no target template — so the locales a run fans out over
// and the units its coverage is derived from can never disagree. Deriving the
// fan-out from the defaults alone left a recipe that names its languages per
// item converging nothing while its coverage counted them.
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

// localeUnitTotals sums each locale's unit count across its coverage scopes —
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
// core path (project.ExtractToBlockStore) — the same implementation behind the
// desktop's Re-extract — and is a full rebuild of the block set (blocks are a
// pure cache; target overlays are preserved). No drift → no work beyond cheap
// stat checks. The returned reason describes the drift for run logs; reporting
// is the caller's job (event stream or plain print).
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
	store := db.Blocks()
	if store == nil {
		return nil, "", fmt.Errorf("extract into the block cache: %w", projectdb.ErrNoStore)
	}
	// Session-transactional, not autocommit: extraction purges the prior block
	// set and refills it, and a half-applied purge is a project with no blocks.
	stats, err := project.ExtractToBlockStore(ctx, a.FormatReg, pctx, store, db, resolved)
	if err != nil {
		return nil, "", err
	}

	// The block set just changed, so the context graph it projects is now stale —
	// rebuild it. This runs AFTER the extraction transaction commits (the write
	// gate is per handle and not reentrant) and searches the freshly written
	// blocks, keeping the occurrence FTS index off the block write path.
	// Best-effort, like the drift stamps above: the graph is a projection, so a
	// failed rebuild only means the next pass rebuilds it, and it must never fail
	// a converge over a derived index.
	if _, gerr := a.MaterializeContextGraph(ctx, layout.Root, pctx.Project); gerr != nil {
		stats.Warnings = append(stats.Warnings, fmt.Sprintf(
			"could not rebuild the context graph: %v — `kapi context` navigation may be stale until the next extract", gerr))
	}
	return &stats, describeDrift(drift), nil
}

// describeDrift renders a short reason for an auto-extract, for the run log.
func describeDrift(d project.StoreDrift) string {
	switch {
	case d.StoreMissing:
		return "nothing extracted yet"
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

// deriveCoverage recomputes per-scope ship coverage from the working tree —
// the same derivation `kapi status` uses, re-read each pass (state is derived,
// never tracked). With withChecks it first runs the project's bound checks over
// the produced units and feeds the failing set into the coverage rollup as an
// exclusion (#1078 G4), returning it so callers can report per-locale counts.
func (a *App) deriveCoverage(ctx context.Context, cmd Command, proj *project.KapiProject, root string, withChecks bool) ([]LocaleCoverage, *CheckExclusions, error) {
	units, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return nil, nil, err
	}
	var excl *CheckExclusions
	if withChecks {
		excl, err = a.computeLoopCheckExclusions(ctx, cmd, proj, root, units)
		if err != nil {
			return nil, nil, err
		}
	}
	cov, err := a.ComputeShipCoverage(ctx, proj, root, units, excl)
	return cov, excl, err
}

// localesNeedingPass returns the locales (in target order) that still have work:
// a scope holding units whose source moved since their translation was decided,
// a gated scope that is not shippable, or — when ungated — content with no
// committed target yet (below the lowest rung).
//
// This is convergence's NEED-based selection, deliberately distinct from the
// applicability-based flow.ResolveFlowLocales that plain flow runs (the
// desktop runner, RunFromProject, RunFlowAllLocales) share: a flow run asks
// "which locales does this flow apply to"; a convergence pass asks "which
// locales are still short of their gate" so converged locales drop out of
// later passes.
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
			// `draft` because it holds a target — so the ungated rung test below
			// reads the scope as complete, and the loop that reported the drift
			// was the same loop that declined to do anything about it. What the
			// unit needs is a translation of the source the project has NOW, which
			// is production: recycle against the new wording, AI for the
			// remainder, exactly as an untranslated unit flows.
			if c.Stale > 0 {
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
			if !c.Gated && c.Pct["draft"] < 100 {
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

// producedUnits is the progress metric: the count of units that have reached at
// least `draft` (any committed target), summed across scopes. A pass that does
// not raise it has stalled.
func producedUnits(cov []LocaleCoverage) int {
	total := 0
	for _, c := range cov {
		total += c.Total * c.Pct["draft"] / 100
	}
	return total
}

// finishConverge derives the final per-locale standing, applies the
// materialize policy (#1078 G2), and emits the structured convergence result.
// It always returns nil for parked work (not a build error); only an
// operational materialize failure is an error.
//
// Materialize policy: when the recipe sets `defaults.materialize: on-converge`
// (or the run forces it with --materialize), every locale whose gated scopes
// are ALL shippable has its target files written from the project block
// store via the shared merge/materialize path; parked locales are skipped —
// their content isn't at the bar yet.
func (a *App) finishConverge(ctx context.Context, cmd Command, proj *project.KapiProject, projectPath, flowName string, passes int, cov []LocaleCoverage, locales []model.LocaleID, sourceGate model.SourceGateLevel, blockedOnSource, totalSource int, facts *convergeFacts, opts ConvergeOptions, emit func(convergence.Event)) error {
	out := buildConvergeOutput(flowName, passes, cov, locales, facts.redraftable)
	out.Monolingual = facts.monolingual
	out.ExtractedFiles = facts.extractedFiles
	out.ExtractedBlocks = facts.extractedBlocks
	out.BlockedOnSource = blockedOnSource
	if sourceGate != "" {
		out.SourceGate = string(sourceGate)
	}
	// Hold-on-source is a first-class outcome (epic 019): when EVERY translatable
	// source block is held (nothing is producible), the run held on SOURCE, not on
	// target review — label it source_not_ready so the summary points at settling
	// the source rather than a review queue, and mark the run un-converged even
	// when no ship gate applies (an all-held ungated run is not "up to date").
	// This mirrors the server's gateItemsBySource: producible==0 && held>0 →
	// source_not_ready. A PARTIAL run (some source ready, some held) translates
	// the ready blocks and reports the held count, but is NOT source_not_ready —
	// the ready work advanced, exactly as the server distinguishes a full hold
	// from a partial one.
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
	// were caught up — the "silent skip → junk output" the source gate exists to
	// prevent. This mirrors the server, which skips its post-run work when the run
	// stalled on source_not_ready (convergence_orchestrator.go). Materialize only
	// once the source is settled and real targets exist.
	// Which translations on disk this run wrote, which is what entitles it to
	// record a basis for them. Under a gate the pass drafts and delivery names
	// the files; ungated, the pass wrote where the recipe points, so a locale it
	// ran for is the answer.
	gated := deliveryIsGated(proj, opts)
	written := writtenTargets{}
	produced := func(locale, _ string) bool { return facts.ranPass(locale) }
	if gated {
		produced = written.wrote
	}

	if gated && out.StallReason != convergence.StallSourceNotReady {
		for i := range out.Locales {
			lc := &out.Locales[i]
			if !lc.Shippable {
				// Parked and pending locales are not delivered. Under this
				// policy that is the whole of delivery — the pass wrote into the
				// run's draft tree — so the locale's files are genuinely absent
				// rather than present and unblessed.
				continue
			}
			// The locale cleared its gate, so its drafts become its delivery.
			// This moves the run's own output, which is the record that exists
			// for every flow; the store-backed write below adds whatever
			// overlays the run also committed, and is a no-op for a flow that
			// committed none.
			delivered, derr := a.deliverDrafts(model.LocaleID(lc.Locale))
			if derr != nil {
				return fmt.Errorf("deliver %s: %w", lc.Locale, derr)
			}
			for _, dest := range delivered {
				written.note(lc.Locale, dest)
			}
			// Per-file progress lines go nowhere: the structured result carries
			// the counts, and stray lines would corrupt --json output.
			n, merr := a.materializeFromProjectStore(ctx, io.Discard, proj, projectPath, []model.LocaleID{model.LocaleID(lc.Locale)}, false)
			if merr != nil {
				return fmt.Errorf("materialize %s: %w", lc.Locale, merr)
			}
			if n < len(delivered) {
				n = len(delivered)
			}
			lc.Materialized = n
			out.MaterializedFiles += n
		}
		if out.MaterializedFiles > 0 {
			emit(convergence.Event{Type: convergence.EventMaterialized, Files: out.MaterializedFiles})
		}

		// Delivery is done, so the drafts stop being anybody's subject and the
		// standing is re-read off the tree the run leaves behind.
		//
		// Everything above is graded against the drafts, correctly: whether a
		// pass moved a locale, and so whether it is delivered, is a question
		// about what the pass produced. What the run REPORTS is a different
		// question — it is what a reader will find, and for a parked locale that
		// is the file already on disk, because its draft was discarded. Reporting
		// the draft's figures put `kapi up` and `kapi status` at odds over one
		// locale with no command between them, which is the whole defect (#2024):
		// a locale whose committed catalog is 97% reviewed and carries a failing
		// check was reported at 3% reviewed and clean, off a tree nobody could
		// open.
		//
		// The verdict does not move by re-reading: a locale is pending — and so
		// parked — because of staleness, a failing check, or a gate it has not
		// cleared, and each of those holds on the delivered file too. What moves
		// is that the numbers become true.
		a.endConvergeDrafts()
		cov, _, rerr := a.deriveCoverage(ctx, cmd, proj, filepath.Dir(projectPath), !opts.noChecks)
		if rerr != nil {
			return fmt.Errorf("re-derive the delivered standing: %w", rerr)
		}
		report := buildConvergeOutput(flowName, passes, cov, locales, facts.redraftable)
		// How many files each locale got is the run's own fact, not the tree's.
		materialized := make(map[string]int, len(out.Locales))
		for _, lc := range out.Locales {
			materialized[lc.Locale] = lc.Materialized
		}
		for i := range report.Locales {
			report.Locales[i].Materialized = materialized[report.Locales[i].Locale]
		}
		out.Locales = report.Locales
		out.ParkedScopes = report.ParkedScopes
		out.Converged = report.Converged
	}

	// Everything this run wrote is the store's own output. Stamping it as
	// absorbed is what keeps the next run from reading its own materialization
	// back as a statement from git; see stampCommittedRecord.
	a.stampCommittedRecord(ctx, proj, projectPath)

	// And the basis for each translation it wrote, so the next run can see a
	// source rewritten under an undecided one. Reported and never fatal: the run
	// has already produced its output, and a state store that could not take the
	// records leaves the following run to write them (host/basisrecord.go).
	if berr := a.recordProducedBasis(ctx, proj, filepath.Dir(projectPath), produced); berr != nil && !a.Quiet {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: could not record what this run translated, so the next run cannot tell "+
				"a rewritten source from a new one: %v\n", berr)
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
// The per-locale figures are read out of the coverage rows and nowhere else —
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
		res := ConvergeLocaleResult{Locale: l, Shippable: true, Verified: true,
			Pct: map[string]int{}, Redrafted: redraftable[l]}
		gatedSomewhere := false
		scoped := false
		for _, c := range cov {
			if c.Locale != l {
				continue
			}
			scoped = true
			for k, v := range c.Pct {
				if v > res.Pct[k] {
					res.Pct[k] = v
				}
			}
			res.Stale += c.Stale
			res.FailingChecks += c.FailingChecks
			// The verdict is the rollup's, unconditionally: a scope that does not
			// ship does not ship, gate or no gate, and reading it only for gated
			// scopes is how a locale held back by stale or failing units still
			// reported itself shippable.
			if !c.Shippable {
				res.Shippable = false
			}
			if !c.Verified {
				res.Verified = false
			}
			if c.Gated {
				gatedSomewhere = true
			}
		}
		// A locale with no coverage row at all has nothing to verify.
		if !scoped {
			res.Verified = false
		}
		if !res.Shippable {
			out.Converged = false
			// Parked is "the loop left work here": short of a gate, or held by a
			// fact — stale wording, a failing guardrail — the loop cannot settle
			// on its own. An ungated locale with neither is simply not gated.
			res.Parked = pendingSet[l] && (gatedSomewhere || res.Stale > 0 || res.FailingChecks > 0)
		}
		out.Locales = append(out.Locales, res)
	}
	// Per-scope parked detail: every (collection, locale) scope that does not
	// ship, in the coverage's stable order — short of its gate, or holding stale
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

// DefaultConvergeFlowSpec returns the built-in default convergence flow used
// when a recipe configures no defaults.flow: content memory reuse (recycle) then
// AI translate over the REMAINDER. It needs no qa step — the convergence loop
// already runs the project's bound checks after each pass (#1078 G4). A fresh
// spec is returned per call so callers can never mutate a shared instance.
//
// skipMatched is what makes the second step mean "the remainder", and it is not
// optional here. The translate tool's own default is to translate every
// translatable block it is handed, which is right for `kapi exec translate` —
// you pointed it at the content — and wrong for a convergence pass, where the
// blocks arrive from the source file with no targets and recycle's fills are the
// only thing standing between a project's own reviewed wording and the model's
// answer. Without it a pass run for ONE pending unit re-translated the whole
// locale, overwriting every recycled target and dropping the approval bound to
// each, and billed a model call per unit for the privilege. The built-in
// `translate` flow (host/flowdef) has always spelled it; this one is the same
// chain and had to say so too.
func DefaultConvergeFlowSpec() *flow.StepsSpec {
	return &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "recycle"},
		{Tool: "translate", Config: map[string]any{"skipMatched": true}},
	}}
}

// prependSourceGate returns a copy of spec with the source-gate leading stage
// (epic 019) inserted at the head — the "leading source-transform stage"
// (CLAUDE.md) that settles the source and holds blocks below gate before any
// producer runs. The gate level rides in the step config so each per-locale
// worker builds a source-gate instance for the resolved level. The input spec
// is not mutated (a fresh spec + steps slice is returned) so a project's shared
// flow definition is never rewritten. A spec that already opens with a
// source-gate step (a recipe that placed it explicitly) is left as-is, so the
// stage is not doubled.
func prependSourceGate(spec *flow.StepsSpec, gate model.SourceGateLevel) *flow.StepsSpec {
	if spec == nil {
		spec = &flow.StepsSpec{}
	}
	if len(spec.Steps) > 0 && spec.Steps[0].Tool == "source-gate" {
		return spec
	}
	steps := make([]flow.FlowStep, 0, len(spec.Steps)+1)
	steps = append(steps, flow.FlowStep{
		Tool:   "source-gate",
		Config: map[string]any{"gate": string(gate)},
	})
	steps = append(steps, spec.Steps...)
	out := *spec
	out.Steps = steps
	return &out
}
