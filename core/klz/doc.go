// Package klz implements the Kapi Localization arcHive (.klz) — a
// ZIP archive containing one or more .klf documents plus skeletons,
// target overlays, vocabulary overrides, annotation sidecars,
// binary assets, and a signed manifest.
//
// Defined by RFC 0001. The archive is the transport and storage
// unit exchanged between extractors, neokapi tools, CI pipelines,
// and external tools. Every authoritative part is JSON or opaque
// skeleton bytes or asset bytes; the format deliberately does not
// store SQLite files, binary indexes, or derived runtime caches.
//
// Package layout mirrors the integration plan in
// neokapi-format/docs/neokapi-integration.md:
//
//   - reader.go: public Reader — iteration + query helpers (query
//     helpers route through internal/db transparently).
//   - writer.go: public Writer — ZIP + manifest + hashing.
//   - manifest.go: manifest schema, SHA-256 integrity checks.
//   - parts.go: document / target / skeleton / asset / annotation
//     part routing, ZIP-slip rejection, size limits.
//   - annotation.go: annotation sidecar I/O (JSON-Lines .klfl).
//   - internal/db: the runtime acceleration cache layer. Scaffold
//     in Phase 1; full SQLite impl behind a `klzcache` build tag
//     in Phase 4. Not importable outside this package tree.
//
// Phase 1 covers the iteration side only. The query side
// (Reader.TM, BlockByID, SimilarSources) and the internal/db
// implementation are deferred to Phase 4.
package klz
