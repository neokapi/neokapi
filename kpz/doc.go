// Package kpz implements the .kpz package container: a portable, lossless
// bundle of an entire neokapi project's authoritative content, assembled from
// the Kapi format family of formats.
//
// A .kpz is a (deterministic) zip with a manifest plus one member per content
// type — the same content-type set the sync protocol enumerates
// (blocks, annotations, tm, terms, media):
//
//	manifest.json                    inventory + per-member sha256 + Merkle rootHash
//	blocks/<id>.kbf.json             core/kbf   (blocks + targets)
//	annotations/<id>.overlays.jsonl  core/kbf   (stand-off annotation overlays)
//	memory.json                      memory/kmb (content memory, lossless)
//	terms.json                       terms/ktb  (terminology, lossless)
//	media/<name>                     opaque blobs
//
// Every member is a *native* Kapi-family format, so the package is lossless:
// unpacking can seed a fresh content memory, terms, and block store and the regenerable
// caches (the parse cache, sync hashes) rebuild faithfully. The package
// deliberately excludes regenerable caches and secrets (nothing from
// `.kapi/work/cache/`, no sync-cache claim
// tokens). It is the at-rest twin of the over-the-wire sync chunk set: pack =
// the sync converters writing files instead of protobuf.
//
// The interchange formats (XLIFF/PO for blocks, TMX for content memory, TBX for terms)
// are a separate, lossy tier for handing content to the wider localization
// industry; they are NOT used as package members because they drop neokapi's
// native fields (entity/concept cross-links, provenance, brand/competitor
// flags, properties).
package kpz
