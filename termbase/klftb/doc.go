// Package klftb implements the KLF-family termbase format (kind
// "kapi-termbase-format"): a deterministic, lossless JSON serialization of a
// neokapi termbase.
//
// It is the termbase analogue of the block format in core/klf. TBX
// (termbase.ExportTBX) is the industry interchange form — lossy, because it
// maps only the standard terminology fields (definition, subject field,
// part-of-speech, gender, administrative status, usage note). klftb is the
// native form that round-trips every field of termbase.Concept, including the
// fields TBX drops: the term Source (terminology vs brand_vocabulary), the
// CompetitorTerm flag, and the extensible Properties map. That losslessness is
// what lets a klftb document seed a fresh termbase exactly, which is why it —
// not TBX — is the termbase member of the .klz package (see package klz).
//
// Since schema version 1.1 the file also carries the concept relations — the
// edges of the brand knowledge graph (termbase.ConceptRelation, AD-021) — in a
// top-level relations array, so a snapshot transports the whole graph, not
// just its nodes. 1.0 files (no relations) remain readable.
//
// The concept model (termbase.Concept) already carries JSON tags, so klftb
// reuses it directly rather than mirroring it in a parallel wire type — one
// source of truth, no drift. The serializer is deterministic: concepts and
// relations sort by id, terms sort, timestamps normalize to UTC, HTML escaping
// is off, and a trailing newline is emitted.
package klftb
