// Package terms provides terminology management following TBX principles.
// A [Terminology] stores language-neutral [Concept] entries, each with [Term]
// values across multiple locales. It supports lookup by source text, domain
// filtering, and brand vocabulary tracking. Implementations include
// [InMemoryStore] and a SQLite-backed store.
package terms
