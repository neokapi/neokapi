package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
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
	// Materialized counts the localized files written for this locale by the
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
	// MaterializedFiles is the total count of localized files written by the
	// post-loop materialize step across every shippable locale (0 when the
	// policy is manual and --materialize was not passed).
	MaterializedFiles int `json:"materializedFiles,omitempty"`
}

// FormatText renders the convergence summary.
func (o ConvergeOutput) FormatText(w io.Writer) error {
	verb := "pass"
	if o.Passes != 1 {
		verb = "passes"
	}
	fmt.Fprintf(w, "Ran flow %q over %d locale(s) in %d %s.\n\n", o.Flow, len(o.Locales), o.Passes, verb)
	for _, lc := range o.Locales {
		state := "pending"
		switch {
		case lc.Parked:
			state = "parked (needs human)"
		case lc.Shippable:
			state = "✓ shippable"
		}
		drafted := lc.Pct["draft"]
		translated := lc.Pct["translated"]
		checks := ""
		if lc.FailingChecks > 0 {
			checks = fmt.Sprintf("  %d failing check(s)", lc.FailingChecks)
		}
		fmt.Fprintf(w, "  %-10s drafted %d%%  translated %d%%  %s%s\n", lc.Locale, drafted, translated, state, checks)
	}
	fmt.Fprintln(w)
	if o.Converged {
		fmt.Fprintln(w, "Converged: every gated scope is shippable.")
	} else {
		fmt.Fprintln(w, "Not fully converged — parked locales await human review (never a build failure).")
	}
	if o.MaterializedFiles > 0 {
		fmt.Fprintf(w, "Materialized %d localized file(s) from the project store.\n", o.MaterializedFiles)
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
		// No defaults.flow: run the built-in default (#1078 G6) — TM reuse then
		// AI translate — so `kapi up` works with zero flow YAML. An explicitly
		// configured defaults.flow always wins over this synthesis.
		flowName = builtinDefaultFlowName
		flowLabel = BuiltinDefaultFlowLabel
		spec = DefaultConvergeFlowSpec()
	} else {
		spec = proj.Flow(flowName)
		if spec == nil {
			if BuiltinComposedFlowNames()[flowName] {
				return fmt.Errorf("defaults.flow %q is a built-in flow; define it under the project's `flows:` map to use it as the convergence default", flowName)
			}
			return fmt.Errorf("default flow %q not found in the project's `flows:`", flowName)
		}
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
		return errors.New("no content to converge (add content patterns to the project)")
	}

	// Standing project context + bindings, so flow steps honor brand-voice /
	// glossary and write to the right per-locale target paths.
	a.ProjectContext = pctx
	defer func() { a.ProjectContext = nil }()
	bindings, err := a.resolveProjectBindings(cmd, proj, projectPath)
	if err != nil {
		return err
	}
	a.ProjectBindings = bindings
	defer func() { a.ProjectBindings = nil }()

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
			err := worker.runProjectStepsOver(ctx, cmd, flowName, spec, &rCtx, sources)
			stopWatch()
			if err != nil {
				return 0, 0, 0, err
			}
			done, viaTM, viaAI := tap.snapshot()
			return done, viaTM, viaAI, nil
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
			if opts.onEvent == nil && !a.Quiet {
				// No event consumer: keep the plain run-log line (bare
				// `kapi run`, embedders listening on LogWriter only).
				fmt.Fprintf(cmd.OutOrStdout(), "Extracted %d block(s) from %d file(s) into the project store (%s).\n",
					stats.Blocks, stats.Files, reason)
			}
			return &convergence.SyncResult{Files: stats.Files, Blocks: stats.Blocks, Reason: reason}, nil
		}
	}

	// Share one parse cache across every pass: unchanged source files parse once,
	// not once per pass; only the targets a pass rewrites re-parse.
	return a.withParseCache(root, func() error {
		res, err := convergence.Loop(ctx, convergence.LoopOptions{
			UntilGate: untilGate,
			MaxPasses: maxPasses,
			Jobs:      jobs,
		}, funcs, emitter)
		if err != nil {
			return err
		}
		d := res.Final.Detail.(derivedState)
		return a.finishConverge(ctx, cmd, proj, projectPath, flowLabel, res.Passes, d.cov, locales, d.excl, opts, emitter.Emit)
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
// re-extracts the project's content into the store when any is found: missing
// store, version-stamped by a different kapi, or source files whose bytes
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
	storePath := layout.BlockStorePath()
	drift := project.DetectStoreDrift(storePath, resolved)
	if !drift.Any() {
		return nil, "", nil
	}
	if err := project.EnsureLayout(layout); err != nil {
		return nil, "", err
	}
	store, err := sqlitestore.New(storePath)
	if err != nil {
		return nil, "", fmt.Errorf("open project block store: %w", err)
	}
	defer store.Close()
	stats, err := project.ExtractToBlockStore(ctx, a.FormatReg, pctx, store, storePath, resolved)
	if err != nil {
		return nil, "", err
	}
	return &stats, describeDrift(drift), nil
}

// describeDrift renders a short reason for an auto-extract, for the run log.
func describeDrift(d project.StoreDrift) string {
	switch {
	case d.StoreMissing:
		return "no block store yet"
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
// are ALL shippable has its localized files written from the project block
// store via the shared merge/materialize path; parked locales are skipped —
// their content isn't at the bar yet.
func (a *App) finishConverge(ctx context.Context, cmd Command, proj *project.KapiProject, projectPath, flowName string, passes int, cov []LocaleCoverage, locales []model.LocaleID, excl *CheckExclusions, opts ConvergeOptions, emit func(convergence.Event)) error {
	out := buildConvergeOutput(flowName, passes, cov, locales, excl)

	if opts.materialize || proj.Defaults.ResolvedMaterialize() == project.MaterializeOnConverge {
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

	state := convergence.RunConverged
	if !out.Converged {
		state = convergence.RunParked
	}
	emit(convergence.Event{Type: convergence.EventDone, State: state})

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
// when a recipe configures no defaults.flow: TM reuse (recycle) followed by AI
// translate. It needs no qa step — the convergence loop already runs the
// project's bound checks after each pass (#1078 G4). A fresh spec is returned
// per call so callers can never mutate a shared instance.
func DefaultConvergeFlowSpec() *flow.StepsSpec {
	return &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "recycle"},
		{Tool: "translate"},
	}}
}
