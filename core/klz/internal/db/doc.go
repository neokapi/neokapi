// Package db is the internal runtime acceleration cache for
// core/klz. It is not importable from outside core/klz (Go's
// internal/ rule), and its schema is not a stable contract.
//
// Phase 1 scaffold: this file tree declares the public types and
// interfaces the Phase-4 SQLite implementation will wire up. The
// actual SQLite code lives behind the `klzcache` build tag and
// lands in Phase 4 per RFC 0001 §Runtime acceleration cache.
//
// Callers should not expect anything in this package to produce
// useful results until Phase 4.
package db
