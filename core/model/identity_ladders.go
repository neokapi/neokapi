package model

// Four ways to say which block this is, and what each one answers.
//
// They are spread across three packages, they disagree with each other on
// purpose, and nothing until now said so. This file changes no behaviour: it is
// the map, placed in core/model because that is the one package all four
// callers already import.
//
// The disagreements are the useful part. Two of these ladders differ in their
// middle rung AND their last resort, and a reader who assumes they are the same
// function under different names will be wrong in a way nothing reports:
//
//	convergence.BlockKey    Unit → Name              → ID
//	model.Block.ChainUnit   Unit → StructuralAddress → Name
//
// Each is right for its own question.
//
// # Address me for storage: convergence.BlockKey
//
// The key the document cache, the overlays and the state store file a block
// under. It ends at the block ID because it must always return something: a row
// has to be addressable even for a block whose format gave it no name. An ID is
// unstable across reads, which is a real cost, and the alternative — a unit that
// cannot be stored — is worse for this question.
//
// # Link my history: model.Block.ChainUnit
//
// What ties a block's successive approved translations into one chain. It
// prefers the structural address over the name because an address is
// translation-invariant while a structural name carries its ancestors' words, so
// the same paragraph is named differently in each document's own language.
//
// It REFUSES to fall through to the ID, and that refusal is the whole difference
// from BlockKey. For a chain an unstable key is worse than no key: empty says
// "this block has no history", which is merely unhelpful, while a wrong key says
// "this block said that before", which is false and will be acted on.
//
// # Am I positioned the same: convergence.BlockAddress
//
// The translation-invariant address alone, empty when the block has none. It is
// what pairs a source file with its translation. Empty is the ordinary case
// rather than a defect: a format whose names are already invariant — a key path,
// an element path, a catalog id — is paired by BlockKey instead.
//
// # Did I transfer: model.BlockIdentity
//
// ContentHash over the text and ContextHash over everything stored beside it.
// Not a name at all: it answers whether a far side already holds what this block
// currently IS, which is a question about content rather than about identity.
//
// # What is missing, and what it costs
//
// There is no join. A block carrying both a structural address and a name is
// filed under the address by its version chain and under the name by the state
// store, so nothing correlates a chain entry with the ship state of the same
// block. A surface wanting to show "this block's history beside where it stands
// today" has to reimplement one ladder in terms of the other.
//
// Unifying them is a data migration, because BlockKey keys persisted state.
// That is why this file is a map and not a refactor: the vocabulary is worth
// fixing now, the stored values are not worth paying for until a surface needs
// the join.
