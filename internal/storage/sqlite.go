// Package storage provides shared SQLite infrastructure for Sievepen (TM)
// and TermBase. Both systems use this layer for connection management,
// WAL mode, and common pragmas.
package storage

import (
	"database/sql"
	"fmt"

	// Pure Go SQLite driver — no CGo dependencies.
	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB with shared configuration applied.
type DB struct {
	*sql.DB
	path string
}

// Open opens a SQLite database at the given path with shared pragmas.
// Use ":memory:" for in-memory databases (useful for testing).
func Open(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", dbPath, err)
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
		"PRAGMA cache_size=-8000", // 8MB cache
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("execute %s: %w", p, err)
		}
	}
	return nil
}
