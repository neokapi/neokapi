// Package ktb implements the Kapi terms format (kind "kapi-terms"), a
// deterministic JSON representation of a terms store.
//
// The format preserves every terms.Concept field, including Source,
// CompetitorTerm and Properties, which TBX export omits. Surface forms appear
// in the forms array; TBX carries them in private x-surfaceForm termNote fields.
// The .kpz package uses ktb so a terms store can be reconstructed without loss.
//
// Schema version 1.1 includes terms.ConceptRelation records in a top-level
// relations array. Version 1.0 files without that array remain readable.
//
// Serialization reuses the concept model's JSON tags. Concepts and relations
// are sorted by ID, terms are sorted, timestamps use UTC, HTML escaping is
// disabled, and the document ends with a newline.
package ktb
