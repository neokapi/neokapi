//go:build !cgo

package storage

import (
	"strings"

	// Pure-Go SQLite (no C compiler required). Used for CGO_ENABLED=0 builds
	// such as the Windows kapi CLI, where mattn/go-sqlite3 registers no driver.
	// modernc ships only the built-in FTS5 tokenizers, so no ICU tokenizer is
	// available here (see FTSWordTokenizer below).
	_ "modernc.org/sqlite"
)

// sqliteDriver is the database/sql driver name registered by modernc.org/sqlite.
const sqliteDriver = "sqlite"

// FTSWordTokenizer is the FTS5 tokenizer used for word-based search tables
// under no-cgo builds. modernc.org/sqlite ships only the FTS5 tokenizers built
// into SQLite itself (unicode61, ascii, porter, trigram); the ICU tokenizer is
// a cgo-only extension. unicode61 gives Unicode-aware word splitting on
// whitespace/punctuation but, unlike ICU, does not segment scripts without
// explicit word boundaries (e.g. CJK, Thai). See driver_cgo.go for the cgo
// counterpart.
const FTSWordTokenizer = "unicode61"

// sqliteDSN builds the modernc.org/sqlite DSN. modernc takes pragmas as
// _pragma=NAME(VALUE) query parameters (e.g. _pragma=busy_timeout(5000),
// _pragma=journal_mode(WAL)); setting busy_timeout in the DSN makes every
// pooled connection wait for locks from the moment it is established.
// In-memory DSNs are left untouched.
func sqliteDSN(dbPath string) string {
	if isMemoryDSN(dbPath) {
		return dbPath
	}
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	return dbPath + sep + "_pragma=busy_timeout(5000)"
}
