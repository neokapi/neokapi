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
	db, err := sql.Open("sqlite3", dbPath)
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

func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
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
