//go:build !cgo && !wasm

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
// _pragma=journal_mode(WAL)), applied to every connection in the pool as it is
// established — this is the only way to guarantee a per-connection pragma
// reaches all up-to-25 pooled connections (a single startup Exec only
// configures the one connection it happens to run on). foreign_keys in
// particular MUST be set here: it is per-connection in SQLite, and TM Delete /
// termbase DeleteConcept rely on ON DELETE CASCADE, which silently no-ops on any
// connection where foreign_keys is OFF.
//
// In-memory DSNs are left untouched.
func sqliteDSN(dbPath string) string {
	if isMemoryDSN(dbPath) {
		return dbPath
	}
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	// journal_mode is intentionally omitted: WAL is a database-level,
	// file-persistent setting that applyPragmas establishes once on first open;
	// re-asserting it on every pooled connection only invites WAL-switch lock
	// contention. The remaining pragmas are per-connection and must ride the DSN.
	params := strings.Join([]string{
		"_pragma=foreign_keys(1)",
		"_pragma=busy_timeout(5000)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=cache_size(-131072)",
		"_pragma=wal_autocheckpoint(10000)",
		"_pragma=temp_store(MEMORY)",
	}, "&")
	return dbPath + sep + params
}
