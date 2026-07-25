// Package memory is neokapi's content memory (content memory) package: the
// [TranslationMemory] interface plus implementations with content-aware
// tiered matching. Unlike traditional Memories that store plain strings, memory
// works with the full content model — Run-based segments with inline codes,
// entity-aware generalized matching, and structural matching that normalizes
// inline codes. Implementations include [InMemoryStore] and the SQLite-backed
// [SQLiteStore].
//
// (If you grepped for "tm" or "content memory": this is that package —
// the name comes from the sieve-and-pen metaphor of filtering candidate
// matches and writing back translations.)
package memory
