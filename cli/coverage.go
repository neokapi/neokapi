package cli

import (
	"context"
	"errors"
	"math"
	"sort"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
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
// state, loaded from the project state store (core/state). A unit reads as
// reviewed/signed-off only while its recorded decision still blesses the CURRENT
// translation — the decision carries the targetHash it approved, so an edit since
// approval invalidates it and the unit drops back to the `translated` baseline.
// This is the authoritative carrier for review state (a plain target file holds
// no status), replacing the old content-keyed .klftm overload — the TM is now the
// recycle corpus only.
type reviewedIndex struct {
	byUnit map[string]reviewedEntry
}

type reviewedEntry struct {
	status     model.TargetStatus
	targetHash string
}

func reviewUnitKey(unit, locale string) string { return unit + "\x00" + locale }

// statusFor returns a block's recorded review state for the locale, or ok=false
// when none applies — including when the recorded decision is stale (the
// translation changed since it was approved).
func (r reviewedIndex) statusFor(b *model.Block, locale string) (model.TargetStatus, bool) {
	if r.byUnit == nil {
		return "", false
	}
	e, ok := r.byUnit[reviewUnitKey(blockKey(b), locale)]
	if !ok {
		return "", false
	}
	if e.targetHash != "" && targetHash(b.TargetText(model.LocaleID(locale))) != e.targetHash {
		return "", false // the translation changed since the decision — stale
	}
	return e.status, true
}

// reviewed reports whether a block is an approved unit (at least reviewed) for the
// locale — used by the review queue to drop approved units.
func (r reviewedIndex) reviewed(b *model.Block, locale string) bool {
	_, ok := r.statusFor(b, locale)
	return ok
}

// upgrade promotes a `translated` unit to its recorded review state (reviewed or
// signed-off) when the block has a non-stale decision for the locale; otherwise it
// returns the base state unchanged.
func (r reviewedIndex) upgrade(base string, b *model.Block, locale string) string {
	if base != string(model.TargetStatusTranslated) {
		return base
	}
	if st, ok := r.statusFor(b, locale); ok {
		return string(st)
	}
	return base
}

// loadReviewedCorrections builds the reviewedIndex from the project state store.
// An absent store yields an empty index (nothing reviewed yet) — never an error,
// so status stays informational.
func (a *App) loadReviewedCorrections(proj *project.KapiProject, root string) (reviewedIndex, error) {
	idx := reviewedIndex{byUnit: map[string]reviewedEntry{}}
	if root == "" {
		return idx, nil
	}
	st, err := openProjectState(proj, root)
	if err != nil {
		return idx, err
	}
	for _, u := range st.All() {
		if u.Status != model.TargetStatusReviewed && u.Status != model.TargetStatusSignedOff {
			continue
		}
		idx.byUnit[reviewUnitKey(u.Unit, string(u.Variant.Locale))] = reviewedEntry{
			status: u.Status, targetHash: u.TargetHash,
		}
	}
	return idx, nil
}

// computeShipCoverage rolls up per-locale coverage over the verify units and
// evaluates each locale against its resolved ship gate. Collection-scoped gate
// rules resolve against (collection, locale); content not in a named collection
// has an empty collection, where the rollup is effectively per-locale.
//
// `reviewed` (loaded from the project's committed .klftm corrections) upgrades a
// unit from the `translated` presence baseline to `reviewed` when its
// source→target pair exactly matches an approved correction — the file-project
// carrier for review state, since a plain target file holds no status.
func (a *App) computeShipCoverage(ctx context.Context, proj *project.KapiProject, root string, units []verifyUnit) ([]LocaleCoverage, error) {
	rs, err := proj.BuildShipGates()
	if err != nil {
		return nil, err
	}
	reviewed, err := a.loadReviewedCorrections(proj, root)
	if err != nil {
		return nil, err
	}

	type scope struct{ collection, locale string }
	statesByScope := map[scope][]string{}
	add := func(s scope, state string) { statesByScope[s] = append(statesByScope[s], state) }

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
			if b.Translatable {
				add(s, reviewed.upgrade(unitState(b, u.locale), b, u.locale))
			}
		}
	}

	ladder := gate.TargetLadder()
	scopes := make([]scope, 0, len(statesByScope))
	for s := range statesByScope {
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
		cov := gate.NewCoverage(statesByScope[s])
		lc := LocaleCoverage{Locale: s.locale, Collection: s.collection, Total: cov.Total, Pct: map[string]int{}}
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
