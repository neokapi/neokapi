package store

// ShipState classifies what a locale (or a collection's locale slice) can ship
// as, and on whose review. It is derived from block counts, never stored.
type ShipState string

const (
	// ShipStateGoverned: fully translated, no failing checks, terms or voice
	// profile rules govern the locale's terminology and every block has a
	// terminology result, and every block's target carries a review decision
	// (human-approved).
	ShipStateGoverned ShipState = "governed"
	// ShipStateApproved: what governed requires, in a locale that neither terms
	// nor voice profile rules govern. A person approved every translation, and
	// only the checks stood behind the approval.
	ShipStateApproved ShipState = "approved"
	// ShipStateAIShippable: fully translated with no failing checks and a
	// terminology result wherever terms govern, but not fully reviewed:
	// shippable on machine review only.
	ShipStateAIShippable ShipState = "ai_shippable"
	// ShipStatePending: anything less: partial coverage, failing checks, a
	// governed block with no terminology result, or nothing to ship yet.
	ShipStatePending ShipState = "pending"
)

// ShipStateInputs are the counts one locale scope's ship state is derived from.
type ShipStateInputs struct {
	TranslatedBlocks int
	TotalBlocks      int
	ApprovedBlocks   int
	FailingChecks    int
	StaleBlocks      int
	RejectedBlocks   int
	// TermsNotCheckedBlocks counts translated blocks in a locale terms govern
	// that have no terminology result.
	TermsNotCheckedBlocks int
	// TermsGoverned reports whether terms or voice profile rules govern the
	// locale's terminology.
	TermsGoverned bool
}

// DeriveShipState derives the ship state for one locale scope from its block
// counts. The dimensions each state depends on:
//
//   - pending when the scope is empty (TotalBlocks == 0), coverage is under
//     100%, or any block fails the checks, is stale, holds wording a reviewer
//     turned down, or has no terminology result in a locale terms govern;
//   - otherwise ai_shippable while approval is incomplete;
//   - otherwise governed when terminology governs the locale, and approved when
//     it does not.
//
// Terminology that governs nothing withholds nothing. A locale with nothing
// bound beyond the checks cannot be called governed, so that is the reason a
// fully approved locale is approved rather than governed, and consumers name
// it. Terminology that governs the locale but has no result for a block
// withholds the scope exactly as a failing check does: a check that did not run
// is not a check that passed.
//
// The voice bar is not a dimension of any state. The worker rewrites voice
// scores on every convergence pass, so a state that read them would move with
// each scoring run rather than with the content. A voice score withholds a
// block from the compliance rate and from approve-passing instead, where a
// person acts on it.
//
// StaleBlocks withholds the scope exactly as FailingChecks does, and for the
// same reason: a unit whose decision blessed source wording the project has
// since rewritten holds a translation of a sentence nobody has, and no machine
// has looked at the new source either. That is not a shortfall of quantity, so
// no coverage arithmetic can offset it.
//
// RejectedBlocks withholds it for the plainest reason of the three: somebody
// read the wording and said no. Nothing has rewritten the source under such a
// unit, so the stale grading reads it as settled, and a scope that shipped it
// would ship a translation on the strength of a machine's opinion over a
// person's (#2564). It clears when the loop drafts something else.
func DeriveShipState(in ShipStateInputs) ShipState {
	state, _ := EvaluateShipState(in)
	return state
}

// EvaluateShipState derives a locale scope's ship state and the gates the state
// was decided on, from one set of counts, so the state a surface serves and the
// quality gate events announced for it cannot disagree. The scope is pending
// exactly when one of the gates is unmet.
//
// Review is not among the gates: a scope that is not fully reviewed ships as
// ai_shippable. The checks and terms gates are evaluated only at full coverage,
// which is where applyShipStates counts them, and terms only where terminology
// governs the locale or a block has no terminology result. A scope below full
// coverage has no result for either, so both are left out.
func EvaluateShipState(in ShipStateInputs) (ShipState, []ShipGateResult) {
	covered := in.TotalBlocks > 0 && in.TranslatedBlocks >= in.TotalBlocks
	gates := []ShipGateResult{{
		Gate:       ShipGateNameTranslated,
		Met:        covered,
		NotChecked: in.TotalBlocks == 0,
		Actual:     in.TranslatedBlocks,
		Required:   in.TotalBlocks,
	}}
	if covered {
		gates = append(gates, ShipGateResult{Gate: ShipGateNameChecks, Met: in.FailingChecks == 0, Actual: in.FailingChecks})
		if in.TermsGoverned || in.TermsNotCheckedBlocks > 0 {
			gates = append(gates, ShipGateResult{
				Gate:       ShipGateNameTerms,
				Met:        in.TermsNotCheckedBlocks == 0,
				NotChecked: in.TermsNotCheckedBlocks > 0,
				Actual:     in.TermsNotCheckedBlocks,
			})
		}
	}
	gates = append(gates,
		ShipGateResult{Gate: ShipGateNameStale, Met: in.StaleBlocks == 0, Actual: in.StaleBlocks},
		ShipGateResult{Gate: ShipGateNameRejected, Met: in.RejectedBlocks == 0, Actual: in.RejectedBlocks},
	)
	for _, g := range gates {
		if !g.Met {
			return ShipStatePending, gates
		}
	}
	switch {
	case in.ApprovedBlocks < in.TotalBlocks:
		return ShipStateAIShippable, gates
	case !in.TermsGoverned:
		return ShipStateApproved, gates
	default:
		return ShipStateGoverned, gates
	}
}
