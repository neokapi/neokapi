// Package storage provides a shared SQLite infrastructure layer for
// persistent translation memories and termbases. It handles connection
// management, WAL mode, and common pragmas.
//
// Two SQLite backends are selected at compile time by the cgo build tag:
//
//   - cgo builds (the default on macOS/Linux dev builds) use the native
//     github.com/mattn/go-sqlite3 driver and the statically linked FTS5 ICU
//     tokenizer (see driver_cgo.go and icu_tokenizer.go), so FTS5 word-search
//     tables use tokenize='icu'.
//   - no-cgo builds (CGO_ENABLED=0, e.g. the Windows kapi CLI) use the pure-Go
//     modernc.org/sqlite driver (see driver_nocgo.go). modernc ships only the
//     built-in FTS5 tokenizers, so FTS5 word-search tables use
//     tokenize='unicode61'.
//
// The driver name (sqliteDriver), DSN builder (sqliteDSN), and word-search
// tokenizer (FTSWordTokenizer) all come from the build-specific driver_*.go
// file.
//
// Cross-build .db caveat: an FTS5 word-search table is created with whichever
// tokenizer the building binary supports. A TM/termbase .db whose FTS table was
// created with tokenize='icu' under a cgo build cannot be FTS-word-queried by a
// no-cgo/modernc binary (which lacks the icu tokenizer), and a db created with
// tokenize='unicode61' under no-cgo cannot rely on ICU segmentation under cgo.
// The trigram tables (tokenize='trigram', built into both backends) and all
// non-FTS data remain portable.
package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DB wraps a sql.DB with shared configuration applied.
type DB struct {
	*sql.DB
	path string
}

// Open opens a SQLite database at the given path with shared pragmas.
// Use ":memory:" for in-memory databases (useful for testing).
// Parent directories must already exist; the file is created on demand.
func Open(dbPath string) (*DB, error) {
	// Set the busy timeout in the DSN so every pooled connection waits for locks
	// from the moment it is established — before any pragma runs. Without this,
	// the very first `PRAGMA journal_mode=WAL` can hit "database is locked" when a
	// second short-lived kapi process (e.g. the verify hook) touches the same DB
	// concurrently, because the per-connection PRAGMA busy_timeout below has not
	// taken effect yet.
	db, err := sql.Open(sqliteDriver, sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", dbPath, err)
	}

	// In-memory databases create a separate DB per connection. Force a single
	// connection so all queries share the same in-memory state.
	if isMemoryDSN(dbPath) {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
	}

	if err := applyPragmas(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}

	return &DB{DB: db, path: dbPath}, nil
}

// Path returns the database file path.
func (db *DB) Path() string {
	return db.path
}

// isMemoryDSN reports whether dbPath addresses an in-memory database. The
// ":memory:" form and "mode=memory" query parameter behave identically across
// the mattn and modernc drivers.
func isMemoryDSN(dbPath string) bool {
	return dbPath == ":memory:" || strings.Contains(dbPath, ":memory:") || strings.Contains(dbPath, "mode=memory")
}

// applyPragmas runs the shared connection pragmas. Both the mattn (cgo) and
// modernc (no-cgo) drivers honour these PRAGMA statements via Exec, so the set
// is kept here rather than in the build-tagged files.
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		// busy_timeout first: subsequent statements (notably the journal_mode=WAL
		// switch, which needs a write lock) then wait for a busy database instead
		// of failing immediately. The DSN sets this per connection too; this is
		// belt-and-suspenders for the connection applyPragmas runs on.
		"PRAGMA busy_timeout=5000",
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA cache_size=-131072",
		"PRAGMA wal_autocheckpoint=10000",
		"PRAGMA temp_store=MEMORY",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil { //nolint:noctx // startup pragmas
			return fmt.Errorf("execute %s: %w", p, err)
		}
	}
	return nil
}
