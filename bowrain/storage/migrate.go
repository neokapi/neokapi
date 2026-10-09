package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Migration represents a single schema migration step.
//
// Migration rule: migrations are APPEND-ONLY. Bowrain databases now carry
// production data, so a migration that has shipped must never be edited,
// renumbered, or removed — schema changes are expressed as a new Migration
// with the next version number (using ALTER TABLE / CREATE ... IF NOT EXISTS
// as appropriate). The per-namespace tracking tables record which versions a
// database has applied; rewriting history desynchronizes them.
type Migration struct {
	Version     int
	Description string
	SQL         string
}

// DefaultMigrationTimeout bounds one migration pass when the caller supplies
// no deadline of its own: Migrate, MigratePostgres and MigratePostgresNS all
// run under it. A pass that cannot finish inside it fails naming the statement
// it was on, instead of waiting until something outside the process (a test
// binary's alarm, an orchestrator's health check) kills it without saying
// where it stalled. A caller expecting a longer pass passes its own context to
// the *Context variant.
const DefaultMigrationTimeout = 2 * time.Minute

// migrationDB is the common surface the migration runner needs from *DB
// (SQLite) and *PgDB (PostgreSQL).
type migrationDB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// runMigrations creates the tracking table with createTableSQL, reads the
// current version from tableName, and applies every migration above it in a
// transaction, recording each with insertSQL (which takes version and
// description as its two parameters in the dialect's placeholder style).
// Every statement runs under ctx, so its deadline bounds the whole pass.
func runMigrations(ctx context.Context, db migrationDB, tableName, createTableSQL, insertSQL string, migrations []Migration) error {
	if _, err := db.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	var currentVersion int
	err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM "+tableName).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}

		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Description, err)
		}

		if _, err := tx.ExecContext(ctx, insertSQL, m.Version, m.Description); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}

	return nil
}

// Migrate applies schema migrations to the SQLite database under
// DefaultMigrationTimeout. It creates a migrations tracking table if it
// doesn't exist, then applies any migrations whose version exceeds the
// current version.
func Migrate(db *DB, migrations []Migration) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultMigrationTimeout)
	defer cancel()
	return MigrateContext(ctx, db, migrations)
}

// MigrateContext is Migrate under the caller's context; its deadline bounds
// the pass.
func MigrateContext(ctx context.Context, db *DB, migrations []Migration) error {
	return runMigrations(ctx, db, "schema_migrations", `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`, "INSERT INTO schema_migrations (version, description) VALUES (?, ?)", migrations)
}
