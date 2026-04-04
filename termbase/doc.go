// Package termbase provides terminology management following TBX principles.
// A [TermBase] stores language-neutral [Concept] entries, each with [Term]
// values across multiple locales. It supports lookup by source text, domain
// filtering, and brand vocabulary tracking. Implementations include
// [InMemoryTermBase] and a SQLite-backed store.
package termbase
