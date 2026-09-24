package store

import "context"

// Ship-gate verdicts are persisted per (block, locale). Computing a verdict
// requires source and target runs, so recomputing dashboard totals on every
// request would require a whole-project read.
//
// Each verdict records its basis: the source hash, target revision and governance
// fingerprint. Aggregates exclude verdicts with a changed basis and report them
// as stale for recomputation. Unchanged projects need only aggregate queries;
// edited projects require reading the affected blocks.
//
// Voice scores are excluded from the basis because draft scoring updates them
// on every convergence pass. Checks and terms determine gate failures. A low
// voice score can exclude a passing block from the compliant count; that
// adjustment uses the scored blocks already available to the pass.

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
	// Terms is the pair's terminology verdict. Fails already holds a
	// violation; Terms is what separates a clean pair whose terminology was
	// checked from one whose locale terms govern but which had nothing to
	// check, such as an empty target, and from one no terms govern.
	Terms TermCompliance
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
	// TermsNotChecked counts blocks that pass the gate in a locale terms govern
	// but carry no terminology verdict, such as an empty target.
	TermsNotChecked int
	// NotChecked counts blocks that pass the gate and are not below a voice bar,
	// yet lack a result for a dimension governing their locale: their
	// terminology was not checked, or a voice profile governs the locale and the
	// block has no score. The compliant count leaves them out.
	NotChecked int
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
	// Scores are the voice scores the pass resolved, the input to the Scored,
	// CleanBelowBar and NotChecked tallies. The pass supplies them only for
	// locales a voice profile governs, since no bar applies anywhere else.
	// Bounded by what has been scored, and carrying no payload beyond the bar
	// comparison.
	Scores []ShipGateScore
	// VoiceGoverned lists the rated locales a voice profile governs. A clean
	// block in one of them with no score is not checked.
	VoiceGoverned []string
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

// ShipGateVoice is what the rollup needs to know about one pair's voice
// standing: whether a voice profile governs its locale, whether the pair has a
// score, and whether that score is under the bar.
type ShipGateVoice struct {
	Governed bool
	Scored   bool
	BelowBar bool
}

// Add folds one verdict into a scope, so a caller that has just computed a
// stale pair can account for it exactly as the query accounted for the rest.
func (r *ShipGateRollup) Add(collectionID, locale string, v ShipGateVerdict, voice ShipGateVoice) {
	if r.Scopes == nil {
		r.Scopes = map[string]map[string]ShipGateCounts{}
	}
	if r.Scopes[collectionID] == nil {
		r.Scopes[collectionID] = map[string]ShipGateCounts{}
	}
	c := r.Scopes[collectionID][locale]
	termsUnchecked := v.Terms == TermComplianceUnchecked
	switch {
	case v.Fails:
		c.Failing++
	default:
		c.Clean++
		if termsUnchecked {
			c.TermsNotChecked++
		}
		switch {
		case voice.BelowBar:
			c.CleanBelowBar++
		case termsUnchecked || (voice.Governed && !voice.Scored):
			c.NotChecked++
		}
	}
	if voice.Scored {
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

// ShipVerdicts finds a verdict store through decorators that expose their wrapped
// store. Wrappers must not implement ShipVerdictStore unconditionally: an empty
// rollup from an unsupported inner store would incorrectly report no failures
// and no stale verdicts.
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
