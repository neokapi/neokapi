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
	if in.TotalBlocks == 0 || in.TranslatedBlocks < in.TotalBlocks || in.FailingChecks > 0 ||
		in.StaleBlocks > 0 || in.RejectedBlocks > 0 || in.TermsNotCheckedBlocks > 0 {
		return ShipStatePending
	}
	if in.ApprovedBlocks < in.TotalBlocks {
		return ShipStateAIShippable
	}
	if !in.TermsGoverned {
		return ShipStateApproved
	}
	return ShipStateGoverned
}
