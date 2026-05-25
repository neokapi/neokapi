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
// underscore-prefixed query parameters; setting _busy_timeout in the DSN makes
// every pooled connection wait for locks from the moment it is established.
// In-memory DSNs are left untouched.
func sqliteDSN(dbPath string) string {
	if isMemoryDSN(dbPath) {
		return dbPath
	}
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	return dbPath + sep + "_busy_timeout=5000"
}
