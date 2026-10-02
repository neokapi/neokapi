//go:build !cgo && !js

package storage

import (
	"strings"

	"github.com/neokapi/neokapi/core/storage/filelock"

	// Pure-Go SQLite (no C compiler required). Used for CGO_ENABLED=0 builds
	// (bare `go build`/`go test` without a C toolchain),
	// where mattn/go-sqlite3 registers no driver. Released binaries are all cgo
	// (see driver_cgo.go). modernc ships only the built-in FTS5 tokenizers, so
	// no ICU tokenizer is available here (see FTSWordTokenizer below).
	_ "modernc.org/sqlite"
)

// sqliteDriver is the database/sql driver name registered by modernc.org/sqlite.
const sqliteDriver = "sqlite"

// driverProfile is what the pure-Go SQLite gives a store: the same as the
// native C build (see driver_cgo.go).
var driverProfile = Profile{
	Driver:           "modernc.org/sqlite",
	MaxConns:         25,
	WAL:              true,
	CrossProcessLock: filelock.Supported,
	Durable:          true,
}

// busyTimeoutMS is how long a connection waits for another connection's lock:
// another thread or process can release it meanwhile.
const busyTimeoutMS = 5000

// cacheSize is the page cache per connection, in the PRAGMA's negative-KiB
// spelling: 128 MiB.
const cacheSize = "-131072"

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
// particular MUST be set here: it is per-connection in SQLite, and content memory Delete /
// terms DeleteConcept rely on ON DELETE CASCADE, which silently no-ops on any
// connection where foreign_keys is OFF.
//
// Options.ImmediateTx is spelled _txlock=immediate, the one DSN parameter
// modernc shares verbatim with mattn (it is not a pragma, so it takes no
// _pragma= wrapper). modernc validates the value and rejects anything but
// deferred, immediate or exclusive.
//
// In-memory DSNs are left untouched.
func sqliteDSN(dbPath string, opts Options) string {
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
	params := []string{
		"_pragma=foreign_keys(1)",
		"_pragma=busy_timeout(5000)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=cache_size(-131072)",
		"_pragma=wal_autocheckpoint(10000)",
		"_pragma=temp_store(MEMORY)",
	}
	if opts.ImmediateTx {
		params = append(params, "_txlock=immediate")
	}
	return dbPath + sep + strings.Join(params, "&")
}
