// Package klz implements the .klz package container: a portable, lossless
// bundle of an entire neokapi project's authoritative content, assembled from
// the KLF family of formats.
//
// A .klz is a (deterministic) zip with a manifest plus one member per content
// type — the same content-type set the sync protocol enumerates
// (blocks, annotations, tm, termbase, media):
//
//	manifest.json          inventory + per-member sha256 + Merkle rootHash
//	blocks/<id>.klf        core/klf            (blocks + targets)
//	annotations/<id>.klfl  core/klf annotations (stand-off overlays)
//	tm.klftm               sievepen/klftm      (translation memory, lossless)
//	termbase.klftb         termbase/klftb      (terminology, lossless)
//	media/<name>           opaque blobs
//
// Every member is a *native* KLF-family format, so the package is lossless:
// unpacking can seed a fresh TM, termbase, and block store and the regenerable
// caches (blocks.db, sync hashes) rebuild faithfully. The package deliberately
// excludes regenerable caches and secrets (no blocks.db, no sync-cache claim
// tokens). It is the at-rest twin of the over-the-wire sync chunk set: pack =
// the sync converters writing files instead of protobuf.
//
// The interchange formats (XLIFF/PO for blocks, TMX for TM, TBX for termbase)
// are a separate, lossy tier for handing content to the wider localization
// industry; they are NOT used as package members because they drop neokapi's
// native fields (entity/concept cross-links, provenance, brand/competitor
// flags, properties).
package klz
