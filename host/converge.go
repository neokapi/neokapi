package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

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
	Shippable bool           `json:"shippable"`        // every gated scope for this locale clears its gate
	Parked    bool           `json:"parked,omitempty"` // still short of its gate after the loop (needs human)
	Pct       map[string]int `json:"pct,omitempty"`    // ladder state → "at least" percent
	// FailingChecks counts units that are produced but fail the project's
	// bound checks (#1078 G4) — they read at `draft`, not `translated`, for
	// gating, so they hold the locale below its gate until fixed.
	FailingChecks int `json:"failingChecks,omitempty"`
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
}

// FormatText renders the convergence summary.
func (o ConvergeOutput) FormatText(w io.Writer) error {
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
		checks := s.Dim("")
		if lc.FailingChecks > 0 {
			checks = s.Error.Render(fmt.Sprintf("%d failing check(s)", lc.FailingChecks))
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
	if a.SourceLang == "" {
		a.SourceLang = string(pctx.SourceLocale)
	}
	if a.SourceLang == "" {
		a.SourceLang = "en"
	}
	locales := pctx.TargetLocales
	if len(locales) == 0 {
		return errors.New("no target languages configured (defaults.target_languages)")
	}

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
	// one group holding every source, bound to the project-wide resolution
	// already made above — one flow run per locale, as convergence has always
	// done.
	groups, err := a.groupInputsByBinding(cmd, proj, pctx.ProjectDir, sources)
	if err != nil {
		return err
	}
	for i := range groups {
		if groups[i].Point.Collection == "" && groups[i].Point.Path == "" {
			groups[i].bindings = bindings
			continue
		}
		b, berr := a.resolveProjectBindings(cmd, proj, projectPath, groups[i].Point)
		if berr != nil {
			return berr
		}
		groups[i].bindings = b
	}

	root := filepath.Dir(projectPath)
	absProjectPath, _ := filepath.Abs(projectPath)
	projectDir := filepath.Dir(absProjectPath)

	savedTarget := a.TargetLang
	defer func() { a.TargetLang = savedTarget }()

	// Convergence materializes the localized target files (not just block-store
	// overlays) so its file-derived coverage sees each pass's output — uniformly
	// across single- and multi-file projects.
	a.convergeWriteFiles = true
	defer func() { a.convergeWriteFiles = false }()

	if maxPasses < 1 {
		maxPasses = 1
	}
	jobs := opts.jobs
	if jobs <= 0 {
		jobs = convergeJobsDefault
	}

	onEvent := opts.onEvent
	emitter := convergence.NewEmitter(onEvent)

	// The venue-neutral loop (core/convergence.Loop) owns the semantics —
	// pass barrier, per-locale fan-out, stall-parks-the-rest; these closures
	// are the CLI venue's IO: working-tree drift re-extract, file-derived
	// coverage with bound checks, and the default flow on per-locale worker
	// Apps.
	derive := func(cov []LocaleCoverage, excl *CheckExclusions) convergence.PassState {
		return convergence.PassState{
			Pending:       localeStrings(localesNeedingPass(cov, locales)),
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
		Produce: func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (int, int, int, error) {
			tap := newConvergeTap(locale)
			worker := a.convergeWorker(locale, tap)
			stopWatch := watchTapProgress(tap, pass, emit.Emit)
			rCtx := flow.ResourceContext{ProjectDir: projectDir, SourceLocale: worker.SourceLang, TargetLocale: locale}
			// One flow run per binding group, sequentially on this locale's
			// worker: each group's bindings are on the worker before its tool
			// chain is assembled. The tap counts across all of them, so the
			// locale's progress is unaffected by how the sources split.
			var err error
			for _, group := range groups {
				worker.ProjectBindings = group.bindings
				if err = worker.runProjectStepsOver(ctx, cmd, flowName, spec, &rCtx, group.Inputs); err != nil {
					break
				}
			}
			stopWatch()
			if err != nil {
				return 0, 0, 0, err
			}
			done, viaMemory, viaAI := tap.snapshot()
			return done, viaMemory, viaAI, nil
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
			held, total, herr := a.settleAndCountHeldSource(ctx, sourceGate, admissionUnits)
			if herr != nil {
				return fmt.Errorf("settle source for the %q gate: %w", sourceGate, herr)
			}
			blockedOnSource, totalSource = held, total
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
		return a.finishConverge(ctx, cmd, proj, projectPath, flowLabel, res.Passes, d.cov, locales, d.excl,
			sourceGate, blockedOnSource, totalSource, opts, emitter.Emit)
	})
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

	// The block set just changed, so the uses_term / in_collection subgraph it
	// projects is now stale — rebuild it. This runs AFTER the extraction
	// transaction commits (the write gate is per handle and not reentrant) and
	// searches the freshly written blocks, keeping the occurrence FTS index off
	// the block write path. Best-effort, like the drift stamps above: the graph
	// is a projection, so a failed rebuild only means the next pass rebuilds it,
	// and it must never fail a converge over a derived index.
	if _, gerr := a.MaterializeUsesTermGraph(ctx, layout.Root); gerr != nil {
		stats.Warnings = append(stats.Warnings, fmt.Sprintf(
			"could not rebuild the term-occurrence graph: %v — `kapi context` term navigation may be stale until the next extract", gerr))
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
		excl, err = a.computeLoopCheckExclusions(ctx, cmd, units)
		if err != nil {
			return nil, nil, err
		}
	}
	cov, err := a.ComputeShipCoverage(ctx, proj, root, units, excl)
	return cov, excl, err
}

// localesNeedingPass returns the locales (in target order) that still have work:
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
func (a *App) finishConverge(ctx context.Context, cmd Command, proj *project.KapiProject, projectPath, flowName string, passes int, cov []LocaleCoverage, locales []model.LocaleID, excl *CheckExclusions, sourceGate model.SourceGateLevel, blockedOnSource, totalSource int, opts ConvergeOptions, emit func(convergence.Event)) error {
	out := buildConvergeOutput(flowName, passes, cov, locales, excl)
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
	}

	// A source-held run produced no translations (epic 019): do NOT materialize.
	// Every locale's source-fallback output would otherwise be written as if it
	// were caught up — the "silent skip → junk output" the source gate exists to
	// prevent. This mirrors the server, which skips its post-run work when the run
	// stalled on source_not_ready (convergence_orchestrator.go). Materialize only
	// once the source is settled and real targets exist.
	if (opts.materialize || proj.Defaults.ResolvedMaterialize() == project.MaterializeOnConverge) &&
		out.StallReason != convergence.StallSourceNotReady {
		for i := range out.Locales {
			lc := &out.Locales[i]
			if !lc.Shippable {
				continue // parked / pending locales do not materialize
			}
			// Per-file progress lines go nowhere: the structured result carries
			// the counts, and stray lines would corrupt --json output.
			n, merr := a.materializeFromProjectStore(ctx, io.Discard, proj, projectPath, []model.LocaleID{model.LocaleID(lc.Locale)}, false)
			if merr != nil {
				return fmt.Errorf("materialize %s: %w", lc.Locale, merr)
			}
			lc.Materialized = n
			out.MaterializedFiles += n
		}
		if out.MaterializedFiles > 0 {
			emit(convergence.Event{Type: convergence.EventMaterialized, Files: out.MaterializedFiles})
		}
	}

	// Everything this run wrote is the store's own output. Stamping it as
	// absorbed is what keeps the next run from reading its own materialization
	// back as a statement from git; see stampCommittedRecord.
	a.stampCommittedRecord(ctx, proj, projectPath)

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
func buildConvergeOutput(flowName string, passes int, cov []LocaleCoverage, locales []model.LocaleID, excl *CheckExclusions) ConvergeOutput {
	out := ConvergeOutput{Flow: flowName, Passes: passes, Converged: true}
	pendingSet := map[string]bool{}
	for _, loc := range localesNeedingPass(cov, locales) {
		pendingSet[string(loc)] = true
	}
	for _, loc := range locales {
		l := string(loc)
		res := ConvergeLocaleResult{Locale: l, Shippable: true, Pct: map[string]int{}, FailingChecks: excl.failingForLocale(l)}
		gatedSomewhere := false
		for _, c := range cov {
			if c.Locale != l {
				continue
			}
			for k, v := range c.Pct {
				if v > res.Pct[k] {
					res.Pct[k] = v
				}
			}
			if c.Gated {
				gatedSomewhere = true
				if !c.Shippable {
					res.Shippable = false
				}
			}
		}
		if gatedSomewhere && !res.Shippable {
			res.Parked = pendingSet[l]
			out.Converged = false
		}
		out.Locales = append(out.Locales, res)
	}
	// Per-scope parked detail: every gated (collection, locale) scope still
	// short of its gate, in the coverage's stable order.
	for _, c := range cov {
		if c.Gated && !c.Shippable {
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
// when a recipe configures no defaults.flow: content memory reuse (recycle) followed by AI
// translate. It needs no qa step — the convergence loop already runs the
// project's bound checks after each pass (#1078 G4). A fresh spec is returned
// per call so callers can never mutate a shared instance.
func DefaultConvergeFlowSpec() *flow.StepsSpec {
	return &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "recycle"},
		{Tool: "translate"},
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
