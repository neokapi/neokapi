// Package sievepen provides translation memory (TM) implementations with
// content-aware tiered matching. Unlike traditional TMs that store plain
// strings, sievepen works with the full content model: Fragments with inline
// spans, entity-aware generalized matching, and structural matching that
// normalizes inline codes. Implementations include [InMemoryTM] and a
// SQLite-backed store.
package sievepen
