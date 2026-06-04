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
// The concept model (termbase.Concept) already carries JSON tags, so klftb
// reuses it directly rather than mirroring it in a parallel wire type — one
// source of truth, no drift. The serializer is deterministic: concepts sort by
// id, terms sort, timestamps normalize to UTC, HTML escaping is off, and a
// trailing newline is emitted.
package klftb
