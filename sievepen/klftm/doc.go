// Package klftm implements the KLF-family translation-memory format
// (kind "kapi-tm-format"): a deterministic, lossless JSON serialization of a
// neokapi translation memory.
//
// It is the TM analogue of the block format in core/klf: where TMX is the
// industry interchange form (lossy — it preserves only the multilingual
// variants a CAT tool understands), klftm is the native form that round-trips
// every field of sievepen.TMEntry — including the entity mappings (with their
// termbase ConceptID cross-links), provenance origins, per-entry properties,
// and notes that TMX silently drops. That losslessness is what lets a klftm
// document seed a fresh TM and reconstruct the project's matching behavior
// exactly, which is why it — not TMX — is the TM member of the .klz package
// (see package klz).
//
// Like core/klf, the serializer is deterministic: entries and import sessions
// are sorted by id, map keys sort, HTML escaping is off, and a trailing newline
// is emitted, so the byte output is stable for content hashing and diffing.
package klftm
