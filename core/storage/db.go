// Package storage provides a shared SQLite infrastructure layer for
// persistent translation memories and termbases. It handles connection
// management, WAL mode, and common pragmas.
//
// The FTS5 ICU tokenizer (from cwt/fts5-icu-tokenizer) is statically
// linked into the binary via icu_tokenizer.go and registered as an
// auto-extension, so `tokenize='icu'` is available in all FTS5 tables.
package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	// Native C SQLite via CGo — significantly faster than the pure-Go
	// transpilation for scan-heavy workloads (facet queries, FTS5, bulk
	// imports on large TMs). Requires a C compiler at build time.
	_ "github.com/mattn/go-sqlite3"
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
	db, err := sql.Open("sqlite3", busyTimeoutDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", dbPath, err)
	}

	// In-memory databases create a separate DB per connection. Force a single
	// connection so all queries share the same in-memory state.
	if dbPath == ":memory:" || strings.Contains(dbPath, "mode=memory") {
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

// busyTimeoutDSN appends mattn/go-sqlite3's _busy_timeout parameter to a file
// DSN so each connection waits (rather than failing) when the database is
// momentarily locked. In-memory DSNs are left untouched.
func busyTimeoutDSN(dbPath string) string {
	if dbPath == ":memory:" || strings.Contains(dbPath, ":memory:") || strings.Contains(dbPath, "mode=memory") {
		return dbPath
	}
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	return dbPath + sep + "_busy_timeout=5000"
}

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
