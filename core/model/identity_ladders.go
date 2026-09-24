package model

// Block identifiers serve different purposes and use different fallback rules.
//
// # Storage key: convergence.BlockKey
//
// BlockKey uses Unit, then Name, then ID. The document cache, overlays and state
// store need a key even when a format supplies no stable name. The final ID
// fallback permits storage but may change across reads.
//
// # History key: model.Block.ChainUnit
//
// ChainUnit uses Unit, then StructuralAddress, then Name. A structural address is
// translation-invariant; a structural name may contain translated ancestor text.
// ChainUnit returns empty when none is available. An unstable ID could associate
// unrelated versions, so it is excluded from the history fallback.
//
// # Source/target pairing: convergence.BlockAddress
//
// BlockAddress returns the translation-invariant structural address, or empty.
// Formats with invariant names, such as key paths and catalog IDs, can pair
// source and target blocks through BlockKey instead.
//
// # Transfer comparison: model.BlockIdentity
//
// ContentHash describes the text; ContextHash describes the accompanying metadata.
// Together they determine whether a receiver already holds the current block.
//
// # Correlating history and state
//
// A block with both a structural address and a name can use different keys in
// history and state. Callers displaying both must account for those fallback
// rules. Changing BlockKey requires a migration of persisted state.
