// Package kmb implements the Kapi-family translation-memory format
// (kind "kapi-tm-format"): a deterministic, lossless JSON serialization of a
// neokapi translation memory.
//
// It is the TM analogue of the block format in core/kbf: where TMX is the
// industry interchange form (lossy — it preserves only the multilingual
// variants a CAT tool understands), kmb is the native form that round-trips
// every field of sievepen.TMEntry — including the entity mappings (with their
// termbase ConceptID cross-links), provenance origins, per-entry properties,
// and notes that TMX silently drops. That losslessness is what lets a kmb
// document seed a fresh TM and reconstruct the project's matching behavior
// exactly, which is why it — not TMX — is the TM member of the .kpz package
// (see package kpz).
//
// Like core/kbf, the serializer is deterministic: entries and import sessions
// are sorted by id, map keys sort, HTML escaping is off, and a trailing newline
// is emitted, so the byte output is stable for content hashing and diffing.
package kmb
