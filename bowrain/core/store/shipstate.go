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
// (coverage 100%), none fails checks, none is stale, and every block's target is
// approved (reviewed or signed-off); ai_shippable when coverage is 100% with no
// failing check and no stale unit but approval is incomplete; pending otherwise.
// An empty scope (totalBlocks == 0) has nothing to ship and is pending.
//
// staleBlocks withholds the scope exactly as failingChecks does, and for the
// same reason: a unit whose decision blessed source wording the project has
// since rewritten holds a translation of a sentence nobody has, and no machine
// has looked at the new source either. That is not a shortfall of quantity, so
// no coverage arithmetic can offset it.
func DeriveShipState(translatedBlocks, totalBlocks, approvedBlocks, failingChecks, staleBlocks int) ShipState {
	if totalBlocks == 0 || translatedBlocks < totalBlocks || failingChecks > 0 || staleBlocks > 0 {
		return ShipStatePending
	}
	if approvedBlocks >= totalBlocks {
		return ShipStateGoverned
	}
	return ShipStateAIShippable
}
