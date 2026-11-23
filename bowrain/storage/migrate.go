package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	corestorage "github.com/neokapi/neokapi/core/storage"
)

// Migration represents a single schema migration step. It is the framework's
// migration (core/storage), so the platform's ledgers and kapi's share one
// identity and one check (corestorage.CheckLedger).
//
// Migration rule: migrations are APPEND-ONLY. Bowrain databases now carry
// production data, so a migration that has shipped must never be edited,
// renumbered, or removed. Schema changes are expressed as a new Migration
// with the next version number (using ALTER TABLE / CREATE ... IF NOT EXISTS
// as appropriate). The runner records each migration's name and identity and
// refuses to start when the code disagrees with what a database recorded.
//
// Apply, when set, runs inside the same transaction after SQL. It carries the
// steps one dialect cannot say in a statement, such as adding a SQLite column
// only when the table lacks it, and declares a Revision.
type Migration = corestorage.Migration

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
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// ledgerSQL is the dialect a ledger is kept in: the statements that create
// the bookkeeping table, add the identity column to one created before it,
// record a migration (version, description, identity) and adopt a row
// recorded before identities (identity, version; the recorded name is kept).
type ledgerSQL struct {
	create, addIdentity, insert, adopt string
}

// runMigrations creates the tracking table with createTableSQL, reads the
// current version from tableName, and applies every migration above it in a
// transaction, recording each with insertSQL (which takes version and
// description as its two parameters in the dialect's placeholder style).
// Every statement runs under ctx, so its deadline bounds the whole pass.
func runMigrations(ctx context.Context, db migrationDB, tableName string, dialect ledgerSQL, migrations []Migration) error {
	ordered, err := corestorage.ValidateMigrations(tableName, migrations)
	if err != nil {
		return err
	}
	if err := corestorage.RecordLedger("bowrain", tableName, ordered); err != nil {
		return err
	}
	if err := corestorage.ShippedCoverage(tableName, ordered, shippedLedgers[tableName]); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, dialect.create); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	if _, err := db.ExecContext(ctx, dialect.addIdentity); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("add the identity column to %s: %w", tableName, err)
	}
	if err := checkAndAdopt(ctx, db, tableName, dialect, ordered); err != nil {
		return err
	}

	var currentVersion int
	err = db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM "+tableName).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	for _, m := range ordered {
		if m.Retired || m.Version <= currentVersion {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}

		if m.SQL != "" {
			if _, err := tx.ExecContext(ctx, corestorage.ExecutableSQL(m)); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Description, err)
			}
		}
		if m.Apply != nil {
			if err := m.Apply(ctx, tx); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Description, err)
			}
		}

		if _, err := tx.ExecContext(ctx, dialect.insert, m.Version, m.Description, corestorage.MigrationIdentity(m)); err != nil {
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
	return MigrateContextNS(ctx, db, "schema_migrations", migrations)
}

// MigrateContextNS is MigrateContext keeping its ledger in tableName. The
// content store keeps schema_migrations, which the committed ledger file pins;
// a list of its own (a test's) names another table.
func MigrateContextNS(ctx context.Context, db *DB, tableName string, migrations []Migration) error {
	if !validTableName.MatchString(tableName) {
		return fmt.Errorf("invalid migration table name: %q", tableName)
	}
	return runMigrations(ctx, db, tableName, ledgerSQL{
		create: `
		CREATE TABLE IF NOT EXISTS ` + tableName + ` (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at  TEXT NOT NULL DEFAULT (datetime('now')),
			identity    TEXT NOT NULL DEFAULT ''
		)`,
		// SQLite has no ADD COLUMN IF NOT EXISTS; the runner reads a
		// "duplicate column" refusal as the column already there.
		addIdentity: "ALTER TABLE " + tableName + " ADD COLUMN identity TEXT NOT NULL DEFAULT ''",
		insert:      "INSERT INTO " + tableName + " (version, description, identity) VALUES (?, ?, ?)",
		adopt:       "UPDATE " + tableName + " SET identity = ? WHERE version = ? AND identity = ''",
	}, migrations)
}

// checkAndAdopt compares the ledger with the code (corestorage.CheckLedger),
// refusing on a disagreement, and records the code's identity on each row
// recorded before identities existed whose name it accepted. The recorded
// name is never overwritten.
func checkAndAdopt(ctx context.Context, db migrationDB, tableName string, dialect ledgerSQL, ordered []Migration) error {
	rows, err := db.QueryContext(ctx, "SELECT version, description, identity FROM "+tableName)
	if err != nil {
		return fmt.Errorf("read %s: %w", tableName, err)
	}
	var applied []corestorage.AppliedMigration
	for rows.Next() {
		var a corestorage.AppliedMigration
		if err := rows.Scan(&a.Version, &a.Description, &a.Identity); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read %s: %w", tableName, err)
		}
		applied = append(applied, a)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("read %s: %w", tableName, err)
	}
	adopt, err := corestorage.CheckLedger(tableName, ordered, applied, shippedLedgers[tableName])
	if err != nil {
		return err
	}
	for _, m := range adopt {
		if _, err := db.ExecContext(ctx, dialect.adopt, corestorage.MigrationIdentity(m), m.Version); err != nil {
			return fmt.Errorf("adopt version %d of %s: %w", m.Version, tableName, err)
		}
	}
	return nil
}
