// Package storage provides shared SQLite infrastructure for Sievepen (TM)
// and TermBase. Both systems use this layer for connection management,
// WAL mode, and common pragmas.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

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
	// Connection-level pragmas (foreign_keys, busy_timeout, synchronous, …) are
	// carried in the DSN so every connection in the pool inherits them as it is
	// established. A startup PRAGMA Exec only configures the single connection it
	// happens to run on; the other up-to-25 pooled connections would otherwise
	// keep SQLite's defaults — foreign_keys=OFF above all, which silently no-ops
	// ON DELETE CASCADE on any connection where it is unset.
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", dbPath, err)
	}

	// In-memory databases create a separate DB per connection. Force a single
	// connection so all queries share the same in-memory state.
	if isMemoryDSN(dbPath) {
		db.SetMaxOpenConns(1)
	} else {
		// Connection pool for file-backed databases.
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
// ":memory:" form and "mode=memory" query parameter behave identically.
func isMemoryDSN(dbPath string) bool {
	return dbPath == ":memory:" || strings.Contains(dbPath, ":memory:") || strings.Contains(dbPath, "mode=memory")
}

// sqliteDSN builds the modernc.org/sqlite DSN. modernc takes pragmas as
// _pragma=NAME(VALUE) query parameters, applied to every connection in the pool
// as it is established — the only way to guarantee a per-connection pragma
// reaches all pooled connections (a single startup Exec only configures the one
// connection it happens to run on). foreign_keys in particular MUST be set here:
// it is per-connection in SQLite, and ON DELETE CASCADE silently no-ops on any
// connection where foreign_keys is OFF.
//
// journal_mode is intentionally omitted: WAL is a database-level, file-persistent
// setting that applyPragmas establishes once on first open; re-asserting it on
// every pooled connection only invites WAL-switch lock contention. In-memory DSNs
// are left untouched.
func sqliteDSN(dbPath string) string {
	if isMemoryDSN(dbPath) {
		return dbPath
	}
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	params := strings.Join([]string{
		"_pragma=foreign_keys(1)",
		"_pragma=busy_timeout(5000)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=cache_size(-8000)", // 8MB cache
	}, "&")
	return dbPath + sep + params
}

// applyPragmas establishes journal_mode=WAL (a database-level setting persisted
// in the DB header, so running it once on first open suffices) and re-asserts the
// connection-level pragmas as belt-and-suspenders.
//
// For file-backed databases the connection-level pragmas (foreign_keys,
// busy_timeout, synchronous, cache_size) ride the DSN (see sqliteDSN) so every
// pooled connection inherits them; the Exec here only configures the single
// connection it happens to run on. For in-memory databases the DSN is left
// untouched (modernc creates a fresh DB per connection), but SetMaxOpenConns(1)
// pins the pool to one connection, so this Exec configures the only connection
// that exists — which is why foreign_keys etc. must be applied here too.
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA cache_size=-8000", // 8MB cache
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(context.Background(), p); err != nil {
			return fmt.Errorf("execute %s: %w", p, err)
		}
	}
	return nil
}
