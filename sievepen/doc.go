// Package sievepen is neokapi's translation memory (TM) package: the
// [TranslationMemory] interface plus implementations with content-aware
// tiered matching. Unlike traditional TMs that store plain strings, sievepen
// works with the full content model — Run-based segments with inline codes,
// entity-aware generalized matching, and structural matching that normalizes
// inline codes. Implementations include [InMemoryTM] and the SQLite-backed
// [SQLiteTM].
//
// (If you grepped for "tm" or "translation memory": this is that package —
// the name comes from the sieve-and-pen metaphor of filtering candidate
// matches and writing back translations.)
package sievepen
