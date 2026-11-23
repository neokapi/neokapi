package convergence

import (
	"encoding/json"
	"errors"
	"math"
	"sort"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
)

// This file is the ONE coverage engine: per-scope state tallying plus the
// ship-gate rollup that turns a tally into LocaleCoverage rows. Venues differ
// only in their block source (the CLI/desktop convergence paths tally from
// working-tree file reads (cli.computeShipCoverage), the desktop status panel
// tallies from the project block store (TallyBlockStore), so both surfaces
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
	cov            gate.Coverage
	stale          int
	staleRedrafted int
	rejectedOwed   int
	rejectedHeld   int
	failingChecks  int
	basisUnknown   int
	// termsNotChecked counts units in a locale the terms govern that have no
	// terminology result.
	termsNotChecked int
}

// CoverageTally accumulates unit states per (collection, locale) scope. Feed
// it with Add from any block source, then read a single scope
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

// Add tallies one unit at the given ladder state ("" = untranslated).
func (t *CoverageTally) Add(s Scope, state string) {
	t.tally(s).cov.Add(state)
}

// AddStale tallies one unit whose recorded basis (a decision's, or the loop's
// own record of the source it translated) names source wording that has since
// changed. It counts at `draft`, because a committed target exists that is not
// a translation of the current source, and again as stale, which withholds the
// scope from shipping.
//
// redrafted says which half of the stale count the unit belongs to. It is
// false while the record still describes the translation on disk, so the loop
// owes the unit a draft against the current source. It is true once something
// has replaced that translation, so the unit waits on a person instead. The
// verdict is the same either way; what differs is who the scope is waiting for.
func (t *CoverageTally) AddStale(s Scope, redrafted bool) {
	st := t.tally(s)
	st.cov.Add(string(model.TargetStatusDraft))
	st.stale++
	if redrafted {
		st.staleRedrafted++
	}
}

// NoteRejectedAwaitingDraft records that a unit carries a reviewer's rejection
// the loop has not drafted against since, without tallying it: the rejection
// already put the unit at `draft` through the accompanying Add, and this names
// what it is waiting for.
//
// It is the local reading of the platform's DecisionBasisTally.RejectedOwed,
// and it is kept apart from the stale count for the same reason: a rejection of
// a translation of the source the project still holds moves neither hash, so no
// grading of the basis can see it.
func (t *CoverageTally) NoteRejectedAwaitingDraft(s Scope) { t.tally(s).rejectedOwed++ }

// NoteRejectedLeftForPerson records a unit whose drafts a reviewer rejected
// as many times as the loop allows (tool.MaxRejections): the loop drafts it
// no more, and a person writes its translation. It is not the loop's work, so
// it is kept apart from RejectedAwaitingDraft, and like it it holds the scope
// out of Shippable.
func (t *CoverageTally) NoteRejectedLeftForPerson(s Scope) { t.tally(s).rejectedHeld++ }

// NoteUnknownBasis records that a unit's record says nothing about the source in
// front of the reader: a decision written before the basis was tracked, or a
// basis the loop recorded for a translation somebody has since rewritten. It
// tallies nothing; the unit is counted at its rung by the accompanying Add,
// and this makes the assumption behind that rung countable.
func (t *CoverageTally) NoteUnknownBasis(s Scope) { t.tally(s).basisUnknown++ }

// NoteFailingCheck records that a unit fails the project's bound checks,
// without tallying it: the unit is counted at its true rung by the accompanying
// Add, and this withholds the scope's verdict.
//
// The unit IS translated (someone may even have reviewed it) so a percentage
// that said otherwise would be a false statement about the content, and the
// same percentage would then differ between a surface that runs the checks and
// one that does not. What a failing check withholds is the verdict, exactly as
// staleness does: a gate is a bar on quantity, and content that fails a
// guardrail is not a shortfall of quantity.
func (t *CoverageTally) NoteFailingCheck(s Scope) { t.tally(s).failingChecks++ }

// NoteTermsNotChecked records that a unit in a locale the project's terms
// govern has no terminology result, without tallying it: the unit is counted
// at its rung by the accompanying Add, and this withholds the scope's verdict
// as a failing check does.
func (t *CoverageTally) NoteTermsNotChecked(s Scope) { t.tally(s).termsNotChecked++ }

// Coverage returns the accumulated distribution for one scope, with ok=false
// when the scope was never tallied.
func (t *CoverageTally) Coverage(s Scope) (gate.Coverage, bool) {
	st, ok := t.tallies[s]
	if !ok {
		return gate.Coverage{}, false
	}
	return st.cov, true
}

// Scopes lists every scope the tally holds, ordered by collection and then
// locale.
func (t *CoverageTally) Scopes() []Scope {
	out := make([]Scope, 0, len(t.tallies))
	for s := range t.tallies {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Collection != out[j].Collection {
			return out[i].Collection < out[j].Collection
		}
		return out[i].Locale < out[j].Locale
	})
	return out
}

// Stale is how many of a scope's units AddStale tallied: counted at `draft`
// in Coverage, with a basis that names source wording the project no longer
// holds.
func (t *CoverageTally) Stale(s Scope) int {
	if st, ok := t.tallies[s]; ok {
		return st.stale
	}
	return 0
}

// Rollup evaluates every tallied scope against the resolved ship gates and
// returns the LocaleCoverage rows, sorted by (locale, collection). Percentages
// are the rounded "at least" values over the target ladder; a scope no gate
// rule matches is not gated. It is RollupGates with no established gate, so no
// scope ships `established`.
func (t *CoverageTally) Rollup(ship gate.RuleSet) []LocaleCoverage {
	return t.RollupGates(ship, gate.RuleSet{})
}

// RollupGates evaluates every tallied scope against BOTH the ship gates and the
// established gates and returns the LocaleCoverage rows, sorted by (locale,
// collection). Both gates read the same tallied distribution over the target
// ladder; they differ only in the bar. A scope no ship rule matches is not
// gated: nothing withholds it, so Shippable reads true. A scope ships
// `established` only where an established gate is declared and cleared.
func (t *CoverageTally) RollupGates(ship, established gate.RuleSet) []LocaleCoverage {
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
			Pct:   map[string]int{},
			Stale: st.stale, FailingChecks: st.failingChecks,
			StaleAwaitingDraft: st.stale - st.staleRedrafted, StaleAwaitingReview: st.staleRedrafted,
			RejectedAwaitingDraft: st.rejectedOwed,
			RejectedLeftForPerson: st.rejectedHeld,
			BasisUnknown:          st.basisUnknown,
			TermsNotChecked:       st.termsNotChecked,
		}
		lc.AtLeast = map[string]int{}
		for _, rung := range ladder {
			n := cov.AtLeastCount(ladder, rung)
			lc.AtLeast[rung] = n
			lc.Pct[rung] = Percent(n, cov.Total)
		}
		if g, ok := ship.Resolve(s.Collection, s.Locale); ok {
			lc.Gated = true
			res := gate.Evaluate(g, cov, ladder)
			lc.Shippable = res.Pass
			lc.Pending = res.Shortfalls
			lc.ShipProgress = res.Progress
			lc.Blocking = res.Blocking
		} else {
			lc.Shippable = true // no ship gate matched this scope; the withholds below still apply
			lc.ShipProgress = 100
		}
		// The established gate is evaluated the same way as the ship gate, over
		// the same distribution. No matching rule means the scope has no
		// established bar, so it does not ship `established`.
		establishedMet := false
		if eg, ok := established.Resolve(s.Collection, s.Locale); ok {
			establishedMet = gate.Evaluate(eg, cov, ladder).Pass
		}
		// Stale content, content a reviewer turned down, and content failing the
		// project's bound checks are withheld whether or not a bar was declared:
		// a project with no gates has nothing else to catch them. A unit the terms govern
		// with no terminology result is withheld too, because nobody checked it.
		//
		// This is the one place the verdict is decided, so every surface reading a
		// LocaleCoverage gets the same answer to "does this ship".
		if lc.Stale > 0 || lc.RejectedAwaitingDraft > 0 || lc.RejectedLeftForPerson > 0 || lc.FailingChecks > 0 || lc.TermsNotChecked > 0 {
			lc.Shippable = false
		}
		lc.ShipState = shipState(lc, establishedMet)
		out = append(out, lc)
	}
	return out
}

// shipState names a rolled-up scope's standing. A scope that does not ship is
// withheld, gate or no gate. One that ships is established when it clears an
// established gate, translated when a ship gate matched it, and not gated when
// neither speaks for it.
func shipState(lc LocaleCoverage, establishedMet bool) ShipState {
	switch {
	case !lc.Shippable:
		return ShipStateWithheld
	case establishedMet:
		return ShipStateEstablished
	case lc.Gated:
		return ShipStateTranslated
	default:
		return ShipStateNotGated
	}
}

// BlockStoreScope describes one collection to tally from a project block
// store: the recipe collection name (the scope key gates resolve against),
// its block-store label (project.CollectionLabel, "(unnamed)" for a bare
// entry), and the target locales to measure.
type BlockStoreScope struct {
	Collection string
	Label      string
	Locales    []string
}

// TallyBlockStore tallies coverage from a project block store session, the
// desktop status panel's block source. For each scope, every translatable
// block counts once per target locale: at `translated` when a committed
// `targets/<locale>` overlay exists for it (the overlay kind `kapi run` /
// `kapi merge` write), else as untranslated (""). An overlay with no source
// stamp counts as untranslated too: an earlier kapi wrote it, and a merge
// writes none of it (blockstore.TargetOverlay.Unstamped). The returned totals map
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
			// (blockstore.StoreKey: the source file's path plus the file-local id).
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
				if o, err := sess.GetOverlay(kind, id); err == nil {
					// A draft an earlier kapi wrote names no source, and a
					// merge writes none of it: the block has no translation.
					var p blockstore.TargetOverlay
					if json.Unmarshal(o.Payload, &p) == nil && p.Unstamped() {
						tally.Add(s, "")
						continue
					}
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

// Percent is n of total as a whole percentage, rounded to the nearest. It
// reads 100 only when every unit is counted and 0 only when none is, so a
// scope with one unit missing never shows as complete. A scope with no units
// is fully covered.
func Percent(n, total int) int {
	if total <= 0 {
		return 100
	}
	p := int(math.Round(float64(n) * 100 / float64(total)))
	switch {
	case p >= 100 && n < total:
		return 99
	case p <= 0 && n > 0:
		return 1
	}
	return p
}
