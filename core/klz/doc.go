// Package klz implements the Kapi Localization arcHive (.klz) — a
// ZIP archive containing one or more .klf documents plus skeletons,
// target overlays, vocabulary overrides, annotation sidecars,
// binary assets, and a signed manifest.
//
// Specified by AD-045. The archive is the transport and storage
// unit exchanged between extractors, neokapi tools, CI pipelines,
// and external tools. Every authoritative part is JSON or opaque
// skeleton bytes or asset bytes; the format deliberately does not
// store SQLite files, binary indexes, or derived runtime caches.
//
// Package layout:
//
//   - reader.go: public Reader — iteration + query helpers (query
//     helpers route through internal/db transparently).
//   - writer.go: public Writer — ZIP + manifest + hashing.
//   - manifest.go: manifest schema, SHA-256 integrity checks.
//   - parts.go: document / target / skeleton / asset / annotation
//     part routing, ZIP-slip rejection, size limits.
//   - annotation.go: annotation sidecar I/O (JSON-Lines .klfl).
//   - cache.go / query.go: runtime cache admin + cached queries.
//   - internal/db: the runtime acceleration cache layer. Active
//     behind the `klzcache` build tag; stubs otherwise. Not
//     importable outside this package tree.
package klz
