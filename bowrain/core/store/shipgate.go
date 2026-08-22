package store

import "context"

// The ship gate's per-block verdict, persisted.
//
// A dashboard load reports, per locale and per collection, how many translated
// blocks fail the ship gate and how many are on brand. Both are aggregates over
// every translated block, and the predicate behind them reads the block's source
// runs beside its target runs — something no SQL expression can decide. Derived
// on the request, that is a whole-project read on every load: the answer is a
// handful of counters, and the work to produce it grows with the customer's
// corpus rather than with the answer.
//
// So the verdict is computed once per (block, locale) and stored beside the
// content it judged, and the dashboard asks the database to COUNT the verdicts.
// A stored verdict names the basis it was computed under — the block's source
// hash, the target row's revision, and a fingerprint of the governance in force
// — so a verdict can never outlive the content or the rules that produced it: a
// row whose basis no longer matches is not counted, it is reported as stale and
// recomputed. Steady state costs two aggregates and no block read; an edited
// corpus costs the blocks that were edited.
//
// Voice scores are deliberately NOT part of the basis. They are written by the
// worker's draft scoring on every convergence pass, and folding them into the
// basis would invalidate every verdict each time the loop ran — which is to say
// it would put the whole-project read back. A voice score cannot change whether
// a block FAILS the gate (that is checks and terms); it can only withhold an
// otherwise-clean block from the compliant count, and that adjustment is applied
// over the scored set, which the pass already holds.

// ShipGateRef names one (block, locale) pair.
type ShipGateRef struct {
	BlockID string
	Locale  string
}

// ShipGateStale is one pair that needs computing, paired with the basis the
// store read for it. Basis is opaque to the caller: it travels back unchanged
// on the verdict, and the store refuses to record a verdict whose basis has
// moved since — so a target rewritten while the pass was reading blocks leaves
// no verdict claiming to have judged it.
type ShipGateStale struct {
	ShipGateRef
	Basis string
}

// ShipGateVerdict is one pair's computed standing against the ship gate, ready
// to be stored: Fails is true when the block's target carries an error-severity
// check finding or breaches terminology governance for the locale.
type ShipGateVerdict struct {
	ShipGateRef
	// Basis is the token that came with the stale pair, returned unchanged.
	Basis string
	Fails bool
}

// ShipGateScore is one scored (block, locale) pair as the voice store
// holds it, reduced to the only question the rollup asks of it: whether the
// score clears the scoring profile's compliance bar.
type ShipGateScore struct {
	ShipGateRef
	BelowBar bool
}

// ShipGateCounts is one scope's tally of stored verdicts. A scope is a
// (collection, locale) pair; the collection id is empty for items that belong
// to none, matching the dashboard's ungrouped bucket.
type ShipGateCounts struct {
	// Failing counts translated blocks whose stored verdict fails the gate.
	Failing int
	// Clean counts translated blocks whose stored verdict passes it.
	Clean int
	// Scored counts clean-or-failing blocks the pass supplied a voice score
	// for, which is what makes the compliance basis able to say voice informed
	// the rate in this scope.
	Scored int
	// CleanBelowBar counts blocks that pass the gate but whose supplied voice
	// score sits under the profile's bar — the ones the compliant count
	// withholds.
	CleanBelowBar int
}

// ShipGateQuery scopes a rollup of stored verdicts.
type ShipGateQuery struct {
	ProjectID string
	Stream    string
	// Gate fingerprints the governance the verdicts must have been computed
	// under. A verdict stored under any other fingerprint is stale.
	Gate string
	// Locales are the target locales the pass rates. A translated pair in any
	// other locale is neither counted nor reported stale.
	Locales []string
	// Scores are the voice scores the pass resolved, the input to the Scored
	// and CleanBelowBar tallies. Bounded by what has been scored, and carrying
	// no payload beyond the bar comparison.
	Scores []ShipGateScore
}

// ShipGateRollup is what one query answers: the per-scope tallies of verdicts
// that still hold, and the pairs whose verdict is missing or was computed
// against content or governance the project no longer holds.
type ShipGateRollup struct {
	// Scopes is keyed by collection id, then by locale.
	Scopes map[string]map[string]ShipGateCounts
	// Stale names the pairs the caller must compute and store. Empty is the
	// steady state: nothing has changed since the last dashboard load.
	Stale []ShipGateStale
}

// CountsFor returns one scope's tally, zero when the scope holds no verdicts.
func (r ShipGateRollup) CountsFor(collectionID, locale string) ShipGateCounts {
	return r.Scopes[collectionID][locale]
}

// Add folds one verdict into a scope, so a caller that has just computed a
// stale pair can account for it exactly as the query accounted for the rest.
func (r *ShipGateRollup) Add(collectionID, locale string, fails, scored, belowBar bool) {
	if r.Scopes == nil {
		r.Scopes = map[string]map[string]ShipGateCounts{}
	}
	if r.Scopes[collectionID] == nil {
		r.Scopes[collectionID] = map[string]ShipGateCounts{}
	}
	c := r.Scopes[collectionID][locale]
	switch {
	case fails:
		c.Failing++
	default:
		c.Clean++
		if belowBar {
			c.CleanBelowBar++
		}
	}
	if scored {
		c.Scored++
	}
	r.Scopes[collectionID][locale] = c
}

// ShipVerdictStore is the persistence half of the ship gate: it counts the
// verdicts a project holds and records the ones a pass has just computed. A
// content store that does not implement it is answered by recomputing every
// pair on every pass — correct, and what the in-memory doubles do.
type ShipVerdictStore interface {
	// ShipGateRollup tallies the stored verdicts that still hold and names the
	// pairs that need recomputing.
	ShipGateRollup(ctx context.Context, q ShipGateQuery) (ShipGateRollup, error)
	// PutShipGateVerdicts records verdicts under the gate fingerprint they were
	// computed with, stamping each with the source hash and target revision it
	// judged so a later rollup can tell whether it still holds.
	PutShipGateVerdicts(ctx context.Context, projectID, stream, gate string, verdicts []ShipGateVerdict) error
}

// ShipVerdicts finds the verdict store behind a content store, looking through
// any decorator that names what it wraps.
//
// The probe has to look through, because a decorator cannot decline an
// interface: a wrapper that forwarded ShipVerdictStore unconditionally would
// answer for an inner store that keeps no verdicts, and the caller — which
// reads an empty rollup as "nothing fails, nothing is stale" — would report a
// project with no failing blocks and no work to do. Answering "the store behind
// me keeps them, ask it" is the only forwarding that cannot lie.
func ShipVerdicts(cs ContentStore) (ShipVerdictStore, bool) {
	for cs != nil {
		if vs, ok := cs.(ShipVerdictStore); ok {
			return vs, true
		}
		inner, ok := cs.(interface{ Unwrap() ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}
