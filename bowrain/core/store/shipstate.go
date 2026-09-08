package store

// ShipState classifies what a locale (or a collection's locale slice) can ship
// as, and on whose review. It is derived from block counts, never stored.
type ShipState string

const (
	// ShipStateGoverned — fully translated, no failing checks, and every
	// block's target carries a review decision (human-approved).
	ShipStateGoverned ShipState = "governed"
	// ShipStateAIShippable — fully translated with no failing checks, but
	// not fully reviewed: shippable on machine review only.
	ShipStateAIShippable ShipState = "ai_shippable"
	// ShipStatePending — anything less: partial coverage, failing checks, or
	// nothing to ship yet.
	ShipStatePending ShipState = "pending"
)

// DeriveShipState derives the ship state for one locale scope from its block
// counts. The rule: governed when every translatable block is translated
// (coverage 100%), none fails checks, none is stale, none holds wording a
// reviewer turned down, and every block's target is approved (reviewed or
// signed-off); ai_shippable when coverage is 100% with none of those and
// approval is incomplete; pending otherwise. An empty scope (totalBlocks == 0)
// has nothing to ship and is pending.
//
// staleBlocks withholds the scope exactly as failingChecks does, and for the
// same reason: a unit whose decision blessed source wording the project has
// since rewritten holds a translation of a sentence nobody has, and no machine
// has looked at the new source either. That is not a shortfall of quantity, so
// no coverage arithmetic can offset it.
//
// rejectedBlocks withholds it for the plainest reason of the three: somebody
// read the wording and said no. Nothing has rewritten the source under such a
// unit, so the stale grading reads it as settled, and a scope that shipped it
// would ship a translation on the strength of a machine's opinion over a
// person's (#2564). It clears when the loop drafts something else.
func DeriveShipState(translatedBlocks, totalBlocks, approvedBlocks, failingChecks, staleBlocks, rejectedBlocks int) ShipState {
	if totalBlocks == 0 || translatedBlocks < totalBlocks || failingChecks > 0 ||
		staleBlocks > 0 || rejectedBlocks > 0 {
		return ShipStatePending
	}
	if approvedBlocks >= totalBlocks {
		return ShipStateGoverned
	}
	return ShipStateAIShippable
}
