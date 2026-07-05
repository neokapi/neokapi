package convergence

import (
	"errors"
	"math"
	"sort"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
)

// This file is the ONE coverage engine: per-scope state tallying plus the
// ship-gate rollup that turns a tally into LocaleCoverage rows. Venues differ
// only in their block source — the CLI/desktop convergence paths tally from
// working-tree file reads (cli.computeShipCoverage), the desktop status panel
// tallies from the project block store (TallyBlockStore) — so both surfaces
// report the same numbers for the same underlying data.

// Scope identifies one (collection, locale) coverage rollup scope. Collection
// is the recipe collection name ("" for content not in a named collection),
// the axis ship-gate rules resolve against.
type Scope struct {
	Collection string
	Locale     string
}

// scopeTally is one scope's accumulated distribution.
type scopeTally struct {
	cov        gate.Coverage
	aiReviewed int
}

// CoverageTally accumulates unit states per (collection, locale) scope. Feed
// it with Add/AddAIDecided from any block source, then read a single scope
// back with Coverage or roll everything up against the project's ship gates
// with Rollup.
type CoverageTally struct {
	tallies map[Scope]*scopeTally
}

// NewCoverageTally returns an empty tally.
func NewCoverageTally() *CoverageTally {
	return &CoverageTally{tallies: map[Scope]*scopeTally{}}
}

func (t *CoverageTally) tally(s Scope) *scopeTally {
	st, ok := t.tallies[s]
	if !ok {
		st = &scopeTally{}
		t.tallies[s] = st
	}
	return st
}

// Add tallies one unit at the given ladder state ("" = untranslated) whose
// state was reached without an AI decision.
func (t *CoverageTally) Add(s Scope, state string) {
	t.tally(s).cov.Add(state)
}

// AddAIDecided tallies one unit whose state was reached by an autonomous AI
// decision ("ai/…" identity): it counts at state in the effective
// distribution and at baseline (the rung it held before the decision,
// typically `translated`) in the human-only one (gate approver classes).
func (t *CoverageTally) AddAIDecided(s Scope, state, baseline string) {
	st := t.tally(s)
	st.cov.AddAIDecided(state, baseline)
	st.aiReviewed++
}

// Coverage returns the accumulated distribution for one scope, with ok=false
// when the scope was never tallied.
func (t *CoverageTally) Coverage(s Scope) (gate.Coverage, bool) {
	st, ok := t.tallies[s]
	if !ok {
		return gate.Coverage{}, false
	}
	return st.cov, true
}

// Rollup evaluates every tallied scope against the resolved ship gates and
// returns the LocaleCoverage rows, sorted by (locale, collection). Percentages
// are the rounded "at least" values over the target ladder; a scope no gate
// rule matches reads as shippable.
func (t *CoverageTally) Rollup(rules gate.RuleSet) []LocaleCoverage {
	ladder := gate.TargetLadder()
	scopes := make([]Scope, 0, len(t.tallies))
	for s := range t.tallies {
		scopes = append(scopes, s)
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].Locale != scopes[j].Locale {
			return scopes[i].Locale < scopes[j].Locale
		}
		return scopes[i].Collection < scopes[j].Collection
	})

	out := make([]LocaleCoverage, 0, len(scopes))
	for _, s := range scopes {
		st := t.tallies[s]
		cov := st.cov
		lc := LocaleCoverage{
			Locale: s.Locale, Collection: s.Collection, Total: cov.Total,
			Pct: map[string]int{}, AIReviewed: st.aiReviewed,
		}
		for _, rung := range ladder {
			lc.Pct[rung] = int(math.Round(cov.AtLeastPct(ladder, rung)))
		}
		if g, ok := rules.Resolve(s.Collection, s.Locale); ok {
			lc.Gated = true
			res := gate.Evaluate(g, cov, ladder)
			lc.Shippable = res.Pass
			lc.Pending = res.Shortfalls
		} else {
			lc.Shippable = true // no gate matched this scope
		}
		out = append(out, lc)
	}
	return out
}

// BlockStoreScope describes one collection to tally from a project block
// store: the recipe collection name (the scope key gates resolve against),
// its block-store label (project.CollectionLabel — "(unnamed)" for a bare
// entry), and the target locales to measure.
type BlockStoreScope struct {
	Collection string
	Label      string
	Locales    []string
}

// TallyBlockStore tallies coverage from a project block store session — the
// desktop status panel's block source. For each scope, every translatable
// block counts once per target locale: at `translated` when a committed
// `targets/<locale>` overlay exists for it (the overlay kind `kapi run` /
// `kapi merge` write), else as untranslated (""). The returned totals map
// carries each collection's translatable block count (by recipe collection
// name), including collections with no target locales, which contribute no
// tally scopes.
func TallyBlockStore(sess blockstore.Session, scopes []BlockStoreScope) (*CoverageTally, map[string]int, error) {
	tally := NewCoverageTally()
	totals := make(map[string]int, len(scopes))
	for _, sc := range scopes {
		translatable := true
		blockIDs := make([]string, 0)
		for b, err := range sess.Blocks(blockstore.BlockFilter{Collection: sc.Label, Translatable: &translatable}) {
			if err != nil {
				return nil, nil, err
			}
			id := b.ID
			if id == "" {
				id = b.Hash
			}
			blockIDs = append(blockIDs, id)
		}
		totals[sc.Collection] = len(blockIDs)

		for _, loc := range sc.Locales {
			s := Scope{Collection: sc.Collection, Locale: loc}
			kind := "targets/" + loc
			for _, id := range blockIDs {
				if _, err := sess.GetOverlay(kind, id); err == nil {
					tally.Add(s, string(model.TargetStatusTranslated))
				} else if errors.Is(err, blockstore.ErrNotFound) {
					tally.Add(s, "")
				} else {
					return nil, nil, err
				}
			}
		}
	}
	return tally, totals, nil
}
