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
	cov           gate.Coverage
	aiReviewed    int
	stale         int
	failingChecks int
	basisUnknown  int
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

// AddStale tallies one unit whose recorded basis — a decision's, or the loop's
// own record of the source it translated — names source wording that has since
// changed. It counts at `draft` — a committed target exists, so the unit is not
// below the ladder, but it is not a translation of the current source either —
// and again as stale, which is what withholds the scope from shipping.
func (t *CoverageTally) AddStale(s Scope) {
	st := t.tally(s)
	st.cov.Add(string(model.TargetStatusDraft))
	st.stale++
}

// NoteUnknownBasis records that a unit's record says nothing about the source in
// front of the reader — a decision written before the basis was tracked, or a
// basis the loop recorded for a translation somebody has since rewritten —
// without tallying it: the unit is counted at its rung by the accompanying Add,
// and this only makes the assumption behind that rung countable.
func (t *CoverageTally) NoteUnknownBasis(s Scope) { t.tally(s).basisUnknown++ }

// NoteFailingCheck records that a unit fails the project's bound checks,
// without tallying it: the unit is counted at its true rung by the accompanying
// Add, and this withholds the scope's verdict.
//
// The unit IS translated — someone may even have reviewed it — so a percentage
// that said otherwise would be a false statement about the content, and the
// same percentage would then differ between a surface that runs the checks and
// one that does not. What a failing check withholds is the verdict, exactly as
// staleness does: a gate is a bar on quantity, and content that fails a
// guardrail is not a shortfall of quantity.
func (t *CoverageTally) NoteFailingCheck(s Scope) { t.tally(s).failingChecks++ }

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
// rule matches reads as shippable. It is RollupGates with no verified gate — the
// verified flag is false everywhere.
func (t *CoverageTally) Rollup(ship gate.RuleSet) []LocaleCoverage {
	return t.RollupGates(ship, gate.RuleSet{})
}

// RollupGates evaluates every tallied scope against BOTH the ship gates and the
// verified gates — the two-gate model — and returns the LocaleCoverage rows,
// sorted by (locale, collection). Both gates read the same tallied distribution
// over the target ladder; they differ only in the bar. A scope no ship rule
// matches reads as shippable (the ship default); a scope no verified rule
// matches reads as NOT verified (the verified default — nothing is verified
// unless a bar is declared and cleared).
func (t *CoverageTally) RollupGates(ship, verified gate.RuleSet) []LocaleCoverage {
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
			Stale: st.stale, FailingChecks: st.failingChecks,
			BasisUnknown: st.basisUnknown,
		}
		for _, rung := range ladder {
			lc.Pct[rung] = int(math.Round(cov.AtLeastPct(ladder, rung)))
		}
		if g, ok := ship.Resolve(s.Collection, s.Locale); ok {
			lc.Gated = true
			res := gate.Evaluate(g, cov, ladder)
			lc.Shippable = res.Pass
			lc.Pending = res.Shortfalls
			lc.ShipProgress = res.Progress
			lc.Blocking = res.Blocking
		} else {
			lc.Shippable = true // no ship gate matched this scope
			lc.ShipProgress = 100
		}
		// The verified gate is evaluated the same way as the ship gate, over the
		// same distribution. No matching rule means the scope has no verified bar,
		// so it reads as unverified (the honest default).
		if vg, ok := verified.Resolve(s.Collection, s.Locale); ok {
			lc.Verified = gate.Evaluate(vg, cov, ladder).Pass
		}
		// Stale content and content failing the project's bound checks are
		// withheld whether or not a bar was declared. An ungated scope reads as
		// shippable because nobody asked for a coverage threshold; nobody asked
		// for wording whose source has been rewritten, or for a translation that
		// drops a placeholder, either — and a project with no gates is exactly the
		// one with nothing else to catch it.
		//
		// This is the one place the verdict is decided, so every surface reading a
		// LocaleCoverage gets the same answer to "does this ship" — which is the
		// whole point of the withholding living here rather than in a percentage
		// one caller demotes and another does not.
		if lc.Stale > 0 || lc.FailingChecks > 0 {
			lc.Shippable = false
			lc.Verified = false
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
			// The block's Hash is the key its overlays are written under
			// (blockstore.StoreKey — the source file's path plus the file-local id).
			// The bare ID restarts in every source file, so asking for it finds
			// nothing and reads as "untranslated" for content that is translated.
			key := b.Hash
			if key == "" {
				key = b.ID
			}
			blockIDs = append(blockIDs, key)
		}
		totals[sc.Collection] = len(blockIDs)

		for _, loc := range sc.Locales {
			s := Scope{Collection: sc.Collection, Locale: loc}
			kind := blockstore.TargetOverlayKind(model.LocaleID(loc))
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
