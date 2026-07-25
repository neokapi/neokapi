// Package kmb implements the Kapi-family content-memory format
// (kind "kapi-memory"): a deterministic, lossless JSON serialization of a
// neokapi content memory.
//
// It is the content memory analogue of the block format in core/kbf: where TMX is the
// industry interchange form (lossy — it preserves only the multilingual
// variants a CAT tool understands), kmb is the native form that round-trips
// every field of memory.Entry — including the entity mappings (with their
// terms ConceptID cross-links), provenance origins, per-entry properties,
// and notes that TMX silently drops. That losslessness is what lets a kmb
// document seed a fresh content memory and reconstruct the project's matching behavior
// exactly, which is why it — not TMX — is the content memory member of the .kpz package
// (see package kpz).
//
// Like core/kbf, the serializer is deterministic: entries and import sessions
// are sorted by id, map keys sort, HTML escaping is off, and a trailing newline
// is emitted, so the byte output is stable for content hashing and diffing.
package kmb
