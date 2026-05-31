//go:build cgo

package storage

import (
	"strings"

	// Native C SQLite via CGo — significantly faster than the pure-Go
	// transpilation for scan-heavy workloads (facet queries, FTS5, bulk
	// imports on large TMs). Requires a C compiler at build time. Pairs with
	// the statically linked FTS5 ICU tokenizer in icu_tokenizer.go.
	_ "github.com/mattn/go-sqlite3"
)

// sqliteDriver is the database/sql driver name registered by mattn/go-sqlite3.
const sqliteDriver = "sqlite3"

// FTSWordTokenizer is the FTS5 tokenizer used for word-based search tables
// under cgo builds. The ICU tokenizer (an extension statically linked via
// icu_tokenizer.go) provides Unicode-aware word segmentation, including CJK and
// Thai. It is not available in pure-Go builds, where unicode61 is used instead
// (see driver_nocgo.go), so this constant is referenced wherever a word-search
// FTS5 table is created.
const FTSWordTokenizer = "icu"

// sqliteDSN builds the mattn/go-sqlite3 DSN. mattn takes pragmas as
// underscore-prefixed query parameters, applied to every connection in the pool
// as it is established — this is the only way to guarantee a per-connection
// pragma reaches all up-to-25 pooled connections (a single startup Exec only
// configures the one connection it happens to run on). foreign_keys in
// particular MUST be set here: it is per-connection in SQLite, and TM Delete /
// termbase DeleteConcept rely on ON DELETE CASCADE, which silently no-ops on any
// connection where foreign_keys is OFF.
//
// mattn exposes dedicated DSN params for these per-connection pragmas
// (_foreign_keys, _busy_timeout, _synchronous, _cache_size). journal_mode is
// deliberately NOT in the DSN: WAL is a database-level, file-persistent setting,
// so applyPragmas sets it once on first open; asserting _journal_mode=WAL on
// every connection as the pool warms up makes them contend for the WAL-switch
// write lock and spuriously fail with "database is locked". wal_autocheckpoint
// and temp_store have no mattn DSN param and stay in applyPragmas; neither
// affects cascade correctness. In-memory DSNs are left untouched.
func sqliteDSN(dbPath string) string {
	if isMemoryDSN(dbPath) {
		return dbPath
	}
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	params := strings.Join([]string{
		"_foreign_keys=1",
		"_busy_timeout=5000",
		"_synchronous=NORMAL",
		"_cache_size=-131072",
	}, "&")
	return dbPath + sep + params
}
