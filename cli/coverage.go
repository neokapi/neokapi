package cli

import (
	"context"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// LocaleCoverage is the ship-gate view for one (collection, locale) scope: the
// state distribution of its translatable units, whether it clears its gate, and
// which thresholds are still pending. Collection is empty for content not in a
// named collection (the common case), where the rows are effectively per-locale.
type LocaleCoverage struct {
	Locale     string           `json:"locale"`
	Collection string           `json:"collection,omitempty"`
	Total      int              `json:"total"`
	Pct        map[string]int   `json:"pct"`               // ladder state → "at least" percent (rounded)
	Gated      bool             `json:"gated"`             // a ship gate applies to this scope
	Shippable  bool             `json:"shippable"`         // gate satisfied (or no gate)
	Pending    []gate.Shortfall `json:"pending,omitempty"` // unmet gate thresholds
}

// SourceCoverage is the source-readiness view for the project: how far its
// source content has progressed along the authoring ladder (authored → checked
// → approved) and whether it clears the optional source gate. Source content is
// shared across all target locales, so this rolls up project-wide over the
// distinct source files (deduped), not per-locale.
type SourceCoverage struct {
	Total     int              `json:"total"`
	Pct       map[string]int   `json:"pct"`               // ladder state → "at least" percent (rounded)
	Gated     bool             `json:"gated"`             // a source gate is configured
	Shippable bool             `json:"shippable"`         // gate satisfied (or no gate)
	Pending   []gate.Shortfall `json:"pending,omitempty"` // unmet gate thresholds
}

// sourceUnitState derives a translatable source block's authoring state. A
// committed Block.SourceStatus is authoritative; otherwise a present, non-empty
// source counts as `authored` (the presence baseline).
func sourceUnitState(b *model.Block) string {
	if strings.TrimSpace(b.SourceText()) == "" {
		return "" // empty source — below every rung
	}
	if b.SourceStatus != "" {
		return string(b.SourceStatus)
	}
	return string(model.SourceStatusAuthored)
}

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

// reviewedIndex is the set of human-approved source→target corrections drawn
// from the project's committed .klftm — the reviewed-translation corpus. The
// committed .klftm is written only by `kapi apply` (a human/agent correction);
// automatic TM write-back lands in the .db cache, never the source. So an exact
// match here means a person signed off on this exact translation: it is the
// file-project carrier for review state (a plain target file holds no status).
type reviewedIndex struct {
	set map[string]struct{} // key: source \x00 target \x00 locale
}

func reviewKey(src, tgt, locale string) string {
	return strings.TrimSpace(src) + "\x00" + strings.TrimSpace(tgt) + "\x00" + locale
}

// reviewed reports whether (src, tgt, locale) is an approved correction.
func (r reviewedIndex) reviewed(src, tgt, locale string) bool {
	if r.set == nil {
		return false
	}
	_, ok := r.set[reviewKey(src, tgt, locale)]
	return ok
}

// upgrade promotes a base coverage state to `reviewed` when the block's
// source→target pair for the locale is an approved correction; otherwise it
// returns the base state unchanged. Only a `translated` unit is upgraded — an
// absent target stays untranslated, and an already-higher state is left alone.
func (r reviewedIndex) upgrade(base string, b *model.Block, locale string) string {
	if base != string(model.TargetStatusTranslated) {
		return base
	}
	if r.reviewed(b.SourceText(), b.TargetText(model.LocaleID(locale)), locale) {
		return string(model.TargetStatusReviewed)
	}
	return base
}

// loadReviewedCorrections builds the reviewedIndex from the project's bound
// .klftm source (defaults.tm_source). An absent or unbound source yields an
// empty index (nothing reviewed yet) — never an error, so status stays
// informational.
func (a *App) loadReviewedCorrections(proj *project.KapiProject, root string) (reviewedIndex, error) {
	idx := reviewedIndex{set: map[string]struct{}{}}
	if proj.Defaults.TMSource == "" || root == "" {
		return idx, nil
	}
	entries, err := loadKLFTMEntries(filepath.Join(root, proj.Defaults.TMSource))
	if err != nil {
		return idx, err
	}
	src := model.LocaleID(a.SourceLang)
	for i := range entries {
		e := &entries[i]
		st := e.VariantText(src)
		if strings.TrimSpace(st) == "" {
			continue
		}
		for loc := range e.Variants {
			if loc == src {
				continue
			}
			tt := e.VariantText(loc)
			if strings.TrimSpace(tt) == "" {
				continue
			}
			idx.set[reviewKey(st, tt, string(loc))] = struct{}{}
		}
	}
	return idx, nil
}

// unitState derives a translatable block's lifecycle state for a locale. When a
// committed status is present (set by a producer / a future store-backed read)
// it is authoritative; otherwise a present, non-empty target counts as
// `translated` (the presence baseline) and an absent/empty target is untranslated.
// Lifecycle beyond translated (reviewed/signed-off) surfaces once per-unit state
// is persisted through interchange/transport — see the convergence model.
func unitState(b *model.Block, locale string) string {
	loc := model.LocaleID(locale)
	t := b.Target(loc)
	if t == nil || strings.TrimSpace(b.TargetText(loc)) == "" {
		return "" // untranslated — below every rung
	}
	if t.Status != "" {
		return string(t.Status)
	}
	return string(model.TargetStatusTranslated)
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
			return nil, berr
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
