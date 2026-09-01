package host

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/state"
)

// computeSourceReadiness rolls up source-authoring readiness over the project's
// distinct source files and evaluates it against the optional source gate. It is
// the source-side counterpart of ComputeShipCoverage; source content is shared
// across locales, so files are deduped by path.
func (a *App) computeSourceReadiness(ctx context.Context, proj *project.KapiProject, root string, units []VerifyUnit) (SourceCoverage, error) {
	g, err := proj.BuildSourceGate()
	if err != nil {
		return SourceCoverage{}, err
	}

	states, _, unreadable, err := a.settleSourceStates(ctx, root, string(proj.Defaults.SourceLanguage), model.SourceGateNone, units)
	if err != nil {
		return SourceCoverage{}, err
	}

	ladder := gate.SourceLadder()
	cov := gate.NewCoverage(states)
	sc := SourceCoverage{Total: cov.Total, Pct: map[string]int{}, Unreadable: unreadable}
	for _, st := range ladder {
		sc.Pct[st] = int(math.Round(cov.AtLeastPct(ladder, st)))
	}
	if len(g) > 0 {
		sc.Gated = true
		res := gate.Evaluate(g, cov, ladder)
		sc.Shippable = res.Pass
		sc.Pending = res.Shortfalls
	} else {
		sc.Shippable = true
	}
	return sc, nil
}

// convergeSourceGate resolves the project's source-first convergence gate level
// (defaults.source_gate) — the level-based gate that governs the CONVERGENCE
// fan-out (`kapi up`), distinct from the coverage-bar SourceGate that `kapi
// check --ship` evaluates. An unset/unknown value resolves to the default
// (checked), so the local venue holds identically to the Bowrain server
// (bowrain/core/store.SourceGateFor). The second result reports whether the raw
// value named a recognized level (false = a typo fell back to the default).
func convergeSourceGate(proj *project.KapiProject) (model.SourceGateLevel, bool) {
	return model.ResolveSourceGate(proj.Defaults.SourceGate)
}

// settleSourceStates settles the project's source-locale blocks and returns each
// translatable unit's rung on the source ladder, plus how many rank below
// gateLevel.
//
// It is the ONE source-axis derivation. The rungs above the `authored` presence
// baseline are reachable from content — `check.SettleSourceStatus` runs the
// provider-free source checks and stamps the terminal readiness rung — so a
// reader that only looked at what the source FILE carries reported the baseline
// for every format with nowhere to write a per-block status, which is most
// catalog formats. Settling here is what lets the source line and the settle
// line of the same run agree, on every format.
//
// A committed status in a format that can carry one still wins: the settle
// leaves an already-approved clean source untouched.
//
// gateLevel == SourceGateNone holds nothing (the convergence opt-out) but still
// settles, because the report is owed either way — a project that turned the
// gate off did not ask to be told its source is unchecked.
// A format with no registered reader is the ONE survivable outcome here, and
// only because it is not a read failure at all: the file was never opened. A
// format can be supplied by a plugin, so a recipe that reads on a machine with
// the plugin installed cannot read on one without it — and a project-wide
// rollup that aborted would make every unrelated collection unreportable
// because one optional dependency is missing. The names are returned, never
// swallowed: coverage measured over content that was never opened reads as
// progress, which is the failure this whole file exists to avoid. Every other
// error still propagates, for the reasons host/converge.go states at the call
// site.
func (a *App) settleSourceStates(ctx context.Context, root, sourceLang string, gateLevel model.SourceGateLevel, units []VerifyUnit) (states []string, held int, unreadable []string, err error) {
	// Committed approvals, seeded onto each block before it settles. Without
	// this the settle derivation only ever produces `authored` or `checked`:
	// check.NewSourceReadinessTool preserves an existing approval, but nothing
	// put one there, so `approved` was a rung the ladder could not reach and
	// `source_gate: approved` held a project's fan-out forever.
	approvals, aerr := a.loadSourceApprovals(ctx, root, sourceLang)
	if aerr != nil {
		return nil, 0, nil, aerr
	}
	docs := a.documentIndexOrEmpty(ctx, root)

	seen := map[string]bool{}
	noReader := map[string]bool{}
	for _, u := range units {
		if seen[u.SourcePath] {
			continue
		}
		seen[u.SourcePath] = true
		blocks, berr := a.readSource(ctx, u)
		if berr != nil {
			if errors.Is(berr, registry.ErrUnknownFormat) {
				noReader[u.SourceFormat] = true
				continue
			}
			return nil, 0, nil, berr
		}
		scope := docs.Scope(root, u.SourcePath)
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			if approvals.approves(scope, blockKey(b), b.SourceText()) {
				b.SourceStatus = model.SourceStatusApproved
			}
			check.SettleSourceStatus(ctx, b)
			states = append(states, sourceUnitState(b))
			if gateLevel != model.SourceGateNone && !gateLevel.Admits(b.SourceStatus) {
				held++
			}
		}
	}
	return states, held, sortedFormatSet(noReader), nil
}

// settleAndCountHeldSource settles the project's source-locale blocks (deduped
// by path) and counts how many translatable source blocks rank below the given
// source gate — the blocked-on-source count `kapi up` surfaces. It is the local,
// file-scan counterpart of the server's settleSource + gate rollup, sharing the
// same core.check settle derivation. A disabled gate (SourceGateNone) settles
// nothing and holds nothing (the opt-out never pays the settlement cost). total
// is how many translatable source blocks were considered.
func (a *App) settleAndCountHeldSource(ctx context.Context, root, sourceLang string, gateLevel model.SourceGateLevel, units []VerifyUnit) (held, total int, unreadable []string, err error) {
	if gateLevel == model.SourceGateNone {
		return 0, 0, nil, nil
	}
	states, held, unreadable, err := a.settleSourceStates(ctx, root, sourceLang, gateLevel, units)
	if err != nil {
		return 0, 0, nil, err
	}
	return held, len(states), unreadable, nil
}

// reviewedIndex maps each unit (document + block identity + locale) to its
// committed review decision, loaded from the project state store (core/state).
// A unit reads at its
// decided rung — reviewed/signed-off for an approval, draft for a rejection —
// only while the recorded decision still judges the CURRENT translation: the
// decision carries the targetHash it applied to, so an edit since the decision
// invalidates it and the unit drops back to the `translated` presence baseline
// (a rejected unit re-enters review once retranslated). This is the
// authoritative carrier for review state (a plain target file holds no status),
// replacing the old content-keyed .memory.json overload — the content memory is now the recycle
// corpus only.
type reviewedIndex struct {
	byUnit map[string]reviewedEntry
	// aiReviews carries advisory AI pre-review annotations (score + model),
	// independent of any decision, so the review queue can display them.
	aiReviews map[string]aiReviewEntry
}

type reviewedEntry struct {
	status     model.TargetStatus
	targetHash string
	// contentHash is the decision's BASIS: the source wording it blessed
	// (state.SourceHash). Empty on a record written before the basis was
	// tracked — unknown, not stale.
	contentHash string
	// by is the recorded decider identity ("" for a plain human decision,
	// "ai/<model>" for an autonomous AI approval, "agent/<client>" for an MCP
	// agent). Gate evaluation distinguishes only the "ai/" prefix.
	by string
}

// blessesTarget reports whether the decision's target half still holds: the
// translation on disk is the one it judged. An unset hash on either side is
// treated as still holding — the same "no content to compare" convention
// state.UnitState.Stale reads an absent hash by.
//
// It is the discriminator between the two ways a decision stops applying, which
// a reader of the record has to tell apart: a decision whose target half still
// holds but whose source moved is a blessing sitting beside a sentence it never
// judged, while one whose target has moved too says nothing at all about what is
// on disk.
func (e reviewedEntry) blessesTarget(b *model.Block, locale model.LocaleID) bool {
	return e.targetHash == "" || targetHash(b.TargetText(locale)) == e.targetHash
}

// basisVerdict grades a recorded decision against the source in front of the
// reader — the derived half of the basis: a decision is a fact and is never
// rewritten, so what a source edit changes is not the record but whether it
// still describes the project.
type basisVerdict int

const (
	// basisNone — no decision is recorded for this unit and locale.
	basisNone basisVerdict = iota
	// basisUnknown — a decision with no basis. It keeps its rung; only the
	// count of such records is reported.
	basisUnknown
	// basisCurrent — the decision blesses the source the project holds now.
	basisCurrent
	// basisStale — the source moved since the decision was recorded.
	basisStale
)

type aiReviewEntry struct {
	score      int
	model      string
	targetHash string
}

// reviewUnitKey is the index's identity: the document a decision was made in,
// the unit it was made about, and the locale it was made for. The document
// belongs in it because a unit id is unique inside its document and nowhere
// wider — without it, one page's decision answered for every page in the
// collection that happened to carry the same id, and every one of them but the
// last read as stale against a source nobody had touched.
func reviewUnitKey(scope, unit, locale string) string {
	return scope + "\x00" + unit + "\x00" + locale
}

// grade looks a block's recorded decision up once and reports everything a
// reader needs from it: the entry, how its basis stands against the CURRENT
// source, and whether the decision still applies at all.
//
// It applies only while both halves of the pairing it blessed still hold — the
// translation it judged, and the source it judged it for. Either having moved
// retires it, and the basis is reported separately because the two demotions
// mean different things to a caller: a moved translation puts the unit back at
// its presence baseline, a moved source says the target no longer translates
// anything the project has.
func (r reviewedIndex) grade(scope string, b *model.Block, locale string) (e reviewedEntry, basis basisVerdict, applies bool) {
	if r.byUnit == nil {
		return reviewedEntry{}, basisNone, false
	}
	e, ok := r.byUnit[reviewUnitKey(scope, blockKey(b), locale)]
	if !ok {
		return reviewedEntry{}, basisNone, false
	}
	switch {
	case e.contentHash == "":
		basis = basisUnknown
	case e.contentHash != state.SourceHash(b.SourceText()):
		basis = basisStale
	default:
		basis = basisCurrent
	}
	if e.targetHash != "" && targetHash(b.TargetText(model.LocaleID(locale))) != e.targetHash {
		return e, basis, false // the translation changed since the decision
	}
	return e, basis, basis != basisStale
}

// entryFor returns a block's applicable review entry for the locale, or
// ok=false when none applies.
func (r reviewedIndex) entryFor(scope string, b *model.Block, locale string) (reviewedEntry, bool) {
	e, _, applies := r.grade(scope, b, locale)
	if !applies {
		return reviewedEntry{}, false
	}
	return e, true
}

// basisFor grades the decision recorded for (block, locale) against the block's
// current source.
func (r reviewedIndex) basisFor(scope string, b *model.Block, locale string) basisVerdict {
	_, basis, _ := r.grade(scope, b, locale)
	return basis
}

// statusFor returns a block's recorded review state for the locale, or ok=false
// when none applies (no decision, or a stale one).
func (r reviewedIndex) statusFor(scope string, b *model.Block, locale string) (model.TargetStatus, bool) {
	e, ok := r.entryFor(scope, b, locale)
	return e.status, ok
}

// decided reports whether a block carries an applicable review decision for the
// locale — approved, signed-off, or rejected. The review queue drops decided
// units: an approved unit is done, a rejected one is back in the work queue,
// and a unit whose pairing has moved is back in the queue too.
func (r reviewedIndex) decided(scope string, b *model.Block, locale string) bool {
	_, ok := r.statusFor(scope, b, locale)
	return ok
}

// approvesTarget reports whether a graded decision is an APPROVAL that still
// holds: reviewed or signed-off, with both halves of the pairing it blessed
// still on disk (which is what `applies` from grade already says).
//
// A rejection is a decision too, and `decided` counts it; it is not an
// endorsement of the translation on disk, so a reader asking "did a person say
// this wording is right?" has to ask separately. It takes the graded entry
// rather than the block so a caller that already graded it does not look the
// same unit up twice.
func approvesTarget(e reviewedEntry, applies bool) bool {
	return applies && (e.status == model.TargetStatusReviewed || e.status == model.TargetStatusSignedOff)
}

// apply moves a `translated` unit to its recorded decision rung — up to reviewed
// or signed-off for an approval, down to draft for a rejection — when the block
// has an applicable decision for the locale; otherwise it returns the base state
// unchanged. aiDecided reports whether the applied rung came from an autonomous
// AI decision ("ai/…" identity), which gate evaluation treats separately, and
// basis is how the recorded decision grades against the current source.
//
// A unit with no target at all grades basisNone whatever the store holds: there
// is no pairing for a decision to have blessed, so there is nothing a source
// edit could invalidate, and reading one would promote an untranslated unit.
func (r reviewedIndex) apply(base, scope string, b *model.Block, locale string) (st string, aiDecided bool, basis basisVerdict) {
	if base == "" {
		return base, false, basisNone
	}
	e, basis, applies := r.grade(scope, b, locale)
	if applies && base == string(model.TargetStatusTranslated) {
		return string(e.status), state.IsAIDecision(e.by), basis
	}
	return base, false, basis
}

// aiReviewFor returns a block's fresh AI pre-review annotation for the locale,
// or ok=false when none was recorded or the translation changed since.
func (r reviewedIndex) aiReviewFor(scope string, b *model.Block, locale string) (aiReviewEntry, bool) {
	if r.aiReviews == nil {
		return aiReviewEntry{}, false
	}
	e, ok := r.aiReviews[reviewUnitKey(scope, blockKey(b), locale)]
	if !ok {
		return aiReviewEntry{}, false
	}
	if e.targetHash != "" && targetHash(b.TargetText(model.LocaleID(locale))) != e.targetHash {
		return aiReviewEntry{}, false
	}
	return e, true
}

// loadReviewedCorrections builds the reviewedIndex from the project state store.
// An absent store yields an empty index (nothing decided yet) — never an error,
// so status stays informational.
func (a *App) loadReviewedCorrections(ctx context.Context, proj *project.KapiProject, root string) (reviewedIndex, error) {
	idx := reviewedIndex{byUnit: map[string]reviewedEntry{}, aiReviews: map[string]aiReviewEntry{}}
	if root == "" {
		return idx, nil
	}
	// No driver-absence special case here: the browser build's store degrades
	// to a JSON-sidecar working set inside the handle, so status answers there
	// with whatever decisions that build recorded rather than with none.
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return idx, err
	}

	all, err := st.All(ctx)
	if err != nil {
		return idx, err
	}
	for _, u := range all {
		switch u.Status {
		case model.TargetStatusReviewed, model.TargetStatusSignedOff, model.TargetStatusDraft:
			idx.byUnit[reviewUnitKey(u.Scope, u.Unit, string(u.Variant.Locale))] = reviewedEntry{
				status: u.Status, targetHash: u.TargetHash,
				contentHash: u.ContentHash, by: u.Decision.By,
			}
		}
		if u.AIReview != nil {
			idx.aiReviews[reviewUnitKey(u.Scope, u.Unit, string(u.Variant.Locale))] = aiReviewEntry{
				score: u.AIReview.Score, model: u.AIReview.Model, targetHash: u.AIReview.TargetHash,
			}
		}
	}
	return idx, nil
}

// ComputeShipCoverage rolls up per-locale coverage over the verify units and
// evaluates each locale against its resolved ship gate. Collection-scoped gate
// rules resolve against (collection, locale); content not in a named collection
// has an empty collection, where the rollup is effectively per-locale.
//
// It is the file-scan block source of the shared coverage engine
// (convergence.CoverageTally): unit states are derived from working-tree file
// reads and fed into the same tally/rollup the desktop's block-store status
// path uses (convergence.TallyBlockStore), so both surfaces report the same
// numbers for the same data.
//
// `reviewed` (loaded from the project state store) upgrades a unit from the
// `translated` presence baseline to its decided rung. Units promoted by an
// autonomous AI decision ("ai/…" identity) are tallied separately: they read as
// reviewed in the display percentages, but a gate's reviewed/signed-off
// threshold only admits them under `by: any` (core/gate approver classes).
//
// `excl` (optional, nil = off) is the check-findings set (#1078 G4): a unit in
// it is produced but failing the project's bound checks. It is counted at its
// true rung — the unit is translated, and may be reviewed — and recorded as a
// failing check, which withholds the scope's ship and verified verdicts. Both
// halves matter: the percentage is a fact about the content and must read the
// same on every surface, while the verdict is what a failing guardrail is
// entitled to withhold.
//
// A caller that passes nil gets honest percentages and a verdict that has not
// been asked the guardrail question, so every surface that publishes a verdict
// supplies it (#2024).
func (a *App) ComputeShipCoverage(ctx context.Context, proj *project.KapiProject, root string, units []VerifyUnit, excl *CheckExclusions) ([]LocaleCoverage, error) {
	rs, err := proj.BuildShipGates()
	if err != nil {
		return nil, err
	}
	vs, err := proj.BuildVerifiedGates()
	if err != nil {
		return nil, err
	}
	tally, err := a.ProjectCoverageTally(ctx, proj, root, units, excl)
	if err != nil {
		return nil, err
	}
	return tally.RollupGates(rs, vs), nil
}

// ProjectCoverageTally derives the coverage tally from working-tree reads: the
// per-(collection, locale) unit states behind everything `kapi status` prints.
//
// ComputeShipCoverage is this plus the gate rollup. The split exists because a
// surface needing ABSOLUTE counts — how many units of this collection are
// translated into this locale — cannot recover them from the rolled-up rows,
// which carry percentages. Deriving them a second way is how two faces come to
// disagree about one project: a target file sitting in the working tree counted
// at the terminal and counted for nothing in an app reading the block store,
// and neither number was wrong for the question its own face was asking.
func (a *App) ProjectCoverageTally(ctx context.Context, proj *project.KapiProject, root string, units []VerifyUnit, excl *CheckExclusions) (*convergence.CoverageTally, error) {
	reviewed, err := a.loadReviewedCorrections(ctx, proj, root)
	if err != nil {
		return nil, err
	}

	tally := convergence.NewCoverageTally()
	docs := a.documentIndexOrEmpty(ctx, root)

	for _, u := range units {
		s := convergence.Scope{Collection: u.Collection, Locale: u.Locale}
		scope := docs.Scope(root, u.SourcePath)
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if !errors.Is(berr, errTargetUnreadable) {
				return nil, berr
			}
			// The target exists but its format can't be read back (a compiled
			// .mo catalog, say). Per-unit measurement is impossible, so count by
			// file-presence: the materialized target stands in for its source
			// units as `translated`.
			srcs, serr := a.readSource(ctx, u)
			if serr != nil {
				return nil, serr
			}
			for _, b := range srcs {
				if b.Translatable {
					tally.Add(s, string(model.TargetStatusTranslated))
				}
			}
			continue
		}
		if missing {
			// No target file yet — every translatable source unit is untranslated.
			srcs, serr := a.readSource(ctx, u)
			if serr != nil {
				return nil, serr
			}
			for _, b := range srcs {
				if b.Translatable {
					tally.Add(s, "")
				}
			}
			continue
		}
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			st, aiDecided, basis := reviewed.apply(unitState(b, u.Locale), scope, b, u.Locale)
			// A unit failing the project's bound checks is recorded as such and
			// then tallied at the rung it actually holds. The finding withholds
			// the scope's verdict (convergence.RollupGates) rather than rewriting
			// what the unit is: a reviewed translation that drops a placeholder is
			// a reviewed translation with a defect, and reporting it as a draft
			// erased a person's decision from the display on the one surface that
			// ran the checks.
			if excl.excluded(u.SourcePath, b, u.Locale) {
				tally.NoteFailingCheck(s)
			}
			// The basis: a decision blessed a specific source, and if that
			// source has been rewritten the translation under it renders a
			// sentence the project no longer has. Derived on every read — the
			// decision itself is history and is never rewritten.
			switch basis {
			case basisStale:
				tally.AddStale(s)
				continue
			case basisUnknown:
				tally.NoteUnknownBasis(s)
			}
			if aiDecided {
				// The AI decision promoted the unit from the `translated`
				// baseline; a human-class threshold sees it there.
				tally.AddAIDecided(s, st, string(model.TargetStatusTranslated))
				continue
			}
			tally.Add(s, st)
		}
	}

	return tally, nil
}

// warnUnreadableFormats tells a human what the rollup could not open. The JSON
// output carries the same names under source.unreadable; this is the line a
// person reading the terminal sees, because a coverage table that silently
// omitted a collection looks exactly like one where that collection is fine.
func (a *App) warnUnreadableFormats(cmd Command, unreadable []string) {
	if a.Quiet {
		return
	}
	for _, f := range unreadable {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: no reader for format %q — content declaring it was not measured; "+
				"install the plugin that supplies it (kapi plugins install %s)\n", f, f)
	}
}

// sortedFormatSet renders a set of format names as a stable, sorted slice. A
// name is empty when the recipe left the format to extension detection, which
// never reaches a plugin format — say so rather than printing "".
func sortedFormatSet(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for f := range set {
		if f == "" {
			f = "(detected by extension)"
		}
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}
