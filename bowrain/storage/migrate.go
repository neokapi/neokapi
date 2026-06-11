package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// Migration represents a single schema migration step.
type Migration struct {
	Version     int
	Description string
	SQL         string
}

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
func runMigrations(db migrationDB, tableName, createTableSQL, insertSQL string, migrations []Migration) error {
	ctx := context.Background()

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

// Migrate applies schema migrations to the SQLite database.
// It creates a migrations tracking table if it doesn't exist,
// then applies any migrations whose version exceeds the current version.
func Migrate(db *DB, migrations []Migration) error {
	return runMigrations(db, "schema_migrations", `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`, "INSERT INTO schema_migrations (version, description) VALUES (?, ?)", migrations)
}
