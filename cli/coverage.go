package cli

import (
	"context"
	"errors"
	"math"
	"sort"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// computeSourceReadiness rolls up source-authoring readiness over the project's
// distinct source files and evaluates it against the optional source gate. It is
// the source-side counterpart of computeShipCoverage; source content is shared
// across locales, so files are deduped by path.
func (a *App) computeSourceReadiness(ctx context.Context, proj *project.KapiProject, units []verifyUnit) (SourceCoverage, error) {
	g, err := proj.BuildSourceGate()
	if err != nil {
		return SourceCoverage{}, err
	}

	seen := map[string]bool{}
	var states []string
	for _, u := range units {
		if seen[u.sourcePath] {
			continue
		}
		seen[u.sourcePath] = true
		blocks, berr := a.readBlocks(ctx, u.sourcePath, a.SourceLang)
		if berr != nil {
			return SourceCoverage{}, berr
		}
		for _, b := range blocks {
			if b.Translatable {
				states = append(states, sourceUnitState(b))
			}
		}
	}

	ladder := gate.SourceLadder()
	cov := gate.NewCoverage(states)
	sc := SourceCoverage{Total: cov.Total, Pct: map[string]int{}}
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

// reviewedIndex maps each unit (block identity + locale) to its committed review
// decision, loaded from the project state store (core/state). A unit reads at its
// decided rung — reviewed/signed-off for an approval, draft for a rejection —
// only while the recorded decision still judges the CURRENT translation: the
// decision carries the targetHash it applied to, so an edit since the decision
// invalidates it and the unit drops back to the `translated` presence baseline
// (a rejected unit re-enters review once retranslated). This is the
// authoritative carrier for review state (a plain target file holds no status),
// replacing the old content-keyed .klftm overload — the TM is now the recycle
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
	// by is the recorded decider identity ("" for a plain human decision,
	// "ai/<model>" for an autonomous AI approval, "agent/<client>" for an MCP
	// agent). Gate evaluation distinguishes only the "ai/" prefix.
	by string
}

type aiReviewEntry struct {
	score      int
	model      string
	targetHash string
}

func reviewUnitKey(unit, locale string) string { return unit + "\x00" + locale }

// entryFor returns a block's recorded review entry for the locale, or ok=false
// when none applies — including when the recorded decision is stale (the
// translation changed since it was approved).
func (r reviewedIndex) entryFor(b *model.Block, locale string) (reviewedEntry, bool) {
	if r.byUnit == nil {
		return reviewedEntry{}, false
	}
	e, ok := r.byUnit[reviewUnitKey(blockKey(b), locale)]
	if !ok {
		return reviewedEntry{}, false
	}
	if e.targetHash != "" && targetHash(b.TargetText(model.LocaleID(locale))) != e.targetHash {
		return reviewedEntry{}, false // the translation changed since the decision — stale
	}
	return e, true
}

// statusFor returns a block's recorded review state for the locale, or ok=false
// when none applies (no decision, or a stale one).
func (r reviewedIndex) statusFor(b *model.Block, locale string) (model.TargetStatus, bool) {
	e, ok := r.entryFor(b, locale)
	return e.status, ok
}

// decided reports whether a block carries a non-stale review decision for the
// locale — approved, signed-off, or rejected. The review queue drops decided
// units: an approved unit is done, a rejected one is back in the work queue.
func (r reviewedIndex) decided(b *model.Block, locale string) bool {
	_, ok := r.statusFor(b, locale)
	return ok
}

// apply moves a `translated` unit to its recorded decision rung — up to reviewed
// or signed-off for an approval, down to draft for a rejection — when the block
// has a non-stale decision for the locale; otherwise it returns the base state
// unchanged. aiDecided reports whether the applied rung came from an autonomous
// AI decision ("ai/…" identity), which gate evaluation treats separately.
func (r reviewedIndex) apply(base string, b *model.Block, locale string) (st string, aiDecided bool) {
	if base != string(model.TargetStatusTranslated) {
		return base, false
	}
	if e, ok := r.entryFor(b, locale); ok {
		return string(e.status), state.IsAIDecision(e.by)
	}
	return base, false
}

// aiReviewFor returns a block's fresh AI pre-review annotation for the locale,
// or ok=false when none was recorded or the translation changed since.
func (r reviewedIndex) aiReviewFor(b *model.Block, locale string) (aiReviewEntry, bool) {
	if r.aiReviews == nil {
		return aiReviewEntry{}, false
	}
	e, ok := r.aiReviews[reviewUnitKey(blockKey(b), locale)]
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
func (a *App) loadReviewedCorrections(proj *project.KapiProject, root string) (reviewedIndex, error) {
	idx := reviewedIndex{byUnit: map[string]reviewedEntry{}, aiReviews: map[string]aiReviewEntry{}}
	if root == "" {
		return idx, nil
	}
	st, err := openProjectState(proj, root)
	if err != nil {
		return idx, err
	}
	for _, u := range st.All() {
		switch u.Status {
		case model.TargetStatusReviewed, model.TargetStatusSignedOff, model.TargetStatusDraft:
			idx.byUnit[reviewUnitKey(u.Unit, string(u.Variant.Locale))] = reviewedEntry{
				status: u.Status, targetHash: u.TargetHash, by: u.Decision.By,
			}
		}
		if u.AIReview != nil {
			idx.aiReviews[reviewUnitKey(u.Unit, string(u.Variant.Locale))] = aiReviewEntry{
				score: u.AIReview.Score, model: u.AIReview.Model, targetHash: u.AIReview.TargetHash,
			}
		}
	}
	return idx, nil
}

// computeShipCoverage rolls up per-locale coverage over the verify units and
// evaluates each locale against its resolved ship gate. Collection-scoped gate
// rules resolve against (collection, locale); content not in a named collection
// has an empty collection, where the rollup is effectively per-locale.
//
// `reviewed` (loaded from the project state store) upgrades a unit from the
// `translated` presence baseline to its decided rung. Units promoted by an
// autonomous AI decision ("ai/…" identity) are tallied separately: they read as
// reviewed in the display percentages, but a gate's reviewed/signed-off
// threshold only admits them under `by: any` (core/gate approver classes).
//
// `excl` (optional, nil = off) is the check-findings exclusion set (#1078 G4):
// a unit in it is produced but failing the project's bound checks, so its state
// demotes to `draft` — it does not count toward the `translated` rung (nor can
// an AI decision promote it) when the gate is evaluated.
func (a *App) computeShipCoverage(ctx context.Context, proj *project.KapiProject, root string, units []verifyUnit, excl *checkExclusions) ([]LocaleCoverage, error) {
	rs, err := proj.BuildShipGates()
	if err != nil {
		return nil, err
	}
	reviewed, err := a.loadReviewedCorrections(proj, root)
	if err != nil {
		return nil, err
	}

	type scope struct{ collection, locale string }
	type scopeTally struct {
		cov        gate.Coverage
		aiReviewed int
	}
	tallies := map[scope]*scopeTally{}
	tally := func(s scope) *scopeTally {
		t, ok := tallies[s]
		if !ok {
			t = &scopeTally{}
			tallies[s] = t
		}
		return t
	}
	add := func(s scope, state string) { tally(s).cov.Add(state) }

	for _, u := range units {
		s := scope{collection: u.collection, locale: u.locale}
		blocks, missing, berr := a.bilingualBlocks(ctx, u)
		if berr != nil {
			if !errors.Is(berr, errTargetUnreadable) {
				return nil, berr
			}
			// The target exists but its format can't be read back (a compiled
			// .mo catalog, say). Per-unit measurement is impossible, so count by
			// file-presence: the materialized target stands in for its source
			// units as `translated`.
			srcs, serr := a.readBlocks(ctx, u.sourcePath, a.SourceLang)
			if serr != nil {
				return nil, serr
			}
			for _, b := range srcs {
				if b.Translatable {
					add(s, string(model.TargetStatusTranslated))
				}
			}
			continue
		}
		if missing {
			// No target file yet — every translatable source unit is untranslated.
			srcs, serr := a.readBlocks(ctx, u.sourcePath, a.SourceLang)
			if serr != nil {
				return nil, serr
			}
			for _, b := range srcs {
				if b.Translatable {
					add(s, "")
				}
			}
			continue
		}
		for _, b := range blocks {
			if !b.Translatable {
				continue
			}
			st, aiDecided := reviewed.apply(unitState(b, u.locale), b, u.locale)
			// Check-findings exclusion wins over any decision: a unit failing
			// the project's bound checks demotes to draft regardless of who
			// approved it.
			if excl.excluded(u.sourcePath, b, u.locale) {
				add(s, demoteFailing(st))
				continue
			}
			if aiDecided {
				t := tally(s)
				// The AI decision promoted the unit from the `translated`
				// baseline; a human-class threshold sees it there.
				t.cov.AddAIDecided(st, string(model.TargetStatusTranslated))
				t.aiReviewed++
				continue
			}
			add(s, st)
		}
	}

	ladder := gate.TargetLadder()
	scopes := make([]scope, 0, len(tallies))
	for s := range tallies {
		scopes = append(scopes, s)
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].locale != scopes[j].locale {
			return scopes[i].locale < scopes[j].locale
		}
		return scopes[i].collection < scopes[j].collection
	})

	out := make([]LocaleCoverage, 0, len(scopes))
	for _, s := range scopes {
		t := tallies[s]
		cov := t.cov
		lc := LocaleCoverage{
			Locale: s.locale, Collection: s.collection, Total: cov.Total,
			Pct: map[string]int{}, AIReviewed: t.aiReviewed,
		}
		for _, st := range ladder {
			lc.Pct[st] = int(math.Round(cov.AtLeastPct(ladder, st)))
		}
		if g, ok := rs.Resolve(s.collection, s.locale); ok {
			lc.Gated = true
			res := gate.Evaluate(g, cov, ladder)
			lc.Shippable = res.Pass
			lc.Pending = res.Shortfalls
		} else {
			lc.Shippable = true // no gate matched this scope
		}
		out = append(out, lc)
	}
	return out, nil
}
