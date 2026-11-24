package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Migration represents a single schema migration step.
//
// Apply, when set, runs inside the migration's transaction after SQL. It
// carries the steps a statement cannot say, such as rewriting stored rows
// with a value computed in Go.
type Migration struct {
	Version     int
	Description string
	SQL         string
	Apply       func(ctx context.Context, tx *sql.Tx) error
	// Revision is the declared revision of a step written in Go (Apply),
	// part of its identity (MigrationIdentity). Change it with the step.
	Revision string
	// Retired marks a version that is spent and no longer applied: a
	// migration removed or folded into a baseline. It carries no work.
	Retired bool
}

// validTableName ensures the namespace only contains safe characters for a table name.
var validTableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// migrateLocks serializes migrations per database file within the process:
// concurrent openers of the same fresh database (converge workers all opening
// the project content memory) would otherwise race to apply the same versions. Keyed by
// path, so unrelated databases never wait on each other; in-memory databases
// are excluded (each is private to its pool).
var migrateLocks pathLocks

// Migrate applies schema migrations to the database using a namespaced migration
// tracking table. Each subsystem (memory, terms) should use a distinct
// namespace to avoid version collisions when sharing a database file.
//
// Concurrent-safe: within the process, migrations of one database file
// serialize on a per-path lock; across processes, each migration re-checks the
// applied version inside its transaction and treats a lost race (another
// process applied it first) as already-done rather than an error.
func Migrate(db *DB, tableName string, migrations []Migration) error {
	if !validTableName.MatchString(tableName) {
		return fmt.Errorf("invalid migration table name: %q", tableName)
	}
	if db.Path() != "" && !isMemoryDSN(db.Path()) {
		l := migrateLocks.get(db.Path())
		l.Lock()
		defer l.Unlock()
	}

	ordered, err := ValidateMigrations(tableName, migrations)
	if err != nil {
		return err
	}
	if err := RecordLedger("core", tableName, ordered); err != nil {
		return err
	}
	if err := ShippedCoverage(tableName, ordered, shippedLedgers[tableName]); err != nil {
		return err
	}
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at  TEXT NOT NULL DEFAULT (datetime('now')),
			identity    TEXT NOT NULL DEFAULT ''
		)
	`, tableName)); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	if err := ensureIdentityColumn(db, tableName); err != nil {
		return err
	}
	if err := checkAndAdopt(db, tableName, ordered); err != nil {
		return err
	}

	var currentVersion int
	err = db.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(version), 0) FROM "+tableName).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	for _, m := range ordered {
		if m.Retired || m.Version <= currentVersion {
			continue
		}
		if err := applyMigration(db, tableName, m); err != nil {
			return err
		}
	}

	return nil
}

// Check compares a ledger with the code without migrating it, for a database
// opened for reading only: it refuses one a newer release migrated, or one
// whose rows disagree with the code (CheckLedger). A ledger table that does
// not exist passes, and nothing is written.
func Check(ctx context.Context, db *DB, tableName string, migrations []Migration) error {
	if !validTableName.MatchString(tableName) {
		return fmt.Errorf("invalid migration table name: %q", tableName)
	}
	ordered, err := ValidateMigrations(tableName, migrations)
	if err != nil {
		return err
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`, tableName).Scan(&exists); err != nil {
		return fmt.Errorf("look for %s: %w", tableName, err)
	}
	if !exists {
		return nil
	}
	var hasIdentity bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pragma_table_info(?) WHERE name = 'identity')`, tableName).Scan(&hasIdentity); err != nil {
		return fmt.Errorf("read the columns of %s: %w", tableName, err)
	}
	query := "SELECT version, description, '' FROM " + tableName
	if hasIdentity {
		query = "SELECT version, description, identity FROM " + tableName
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("read %s: %w", tableName, err)
	}
	var applied []AppliedMigration
	for rows.Next() {
		var a AppliedMigration
		if err := rows.Scan(&a.Version, &a.Description, &a.Identity); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read %s: %w", tableName, err)
		}
		applied = append(applied, a)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("read %s: %w", tableName, err)
	}
	_, err = CheckLedger(tableName, ordered, applied, shippedLedgers[tableName])
	return err
}

// errLostRace marks the one non-lock condition applyMigration retries: another
// applier committed this migration's version row between our in-transaction
// re-check and our own insert. Only the version-row INSERT raises it. A UNIQUE
// violation from a migration's *own* statements is a real fault, retrying it
// for the whole window would bury the actual error under a timeout, so the
// retry predicate tests this sentinel rather than the error text.
var errLostRace = errors.New("migration version already recorded by a concurrent applier")

// applyMigration applies one migration, tolerating a concurrent applier in
// another process: the version is re-checked inside the transaction, a lost
// race on the version row retries and then reads as already-applied, and
// transient lock contention retries within a bounded window.
func applyMigration(db *DB, tableName string, m Migration) error {
	const window = 15 * time.Second
	delay := 10 * time.Millisecond
	deadline := time.Now().Add(window)
	for {
		applied, err := tryApplyMigration(db, tableName, m)
		if err == nil {
			_ = applied
			return nil
		}
		retryable := isBusyErr(err) || errors.Is(err, errLostRace)
		if !retryable || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		if delay < 500*time.Millisecond {
			delay *= 2
		}
	}
}

// tryApplyMigration runs one attempt: begin, re-check the version inside the
// transaction (another applier may have won since the caller's read), apply,
// record, commit. Returns (false, nil) when the migration turned out to be
// already applied.
func tryApplyMigration(db *DB, tableName string, m Migration) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin migration %d: %w", m.Version, err)
	}

	var cur int
	if err := tx.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(version), 0) FROM "+tableName).Scan(&cur); err != nil {
		_ = tx.Rollback()
		return false, fmt.Errorf("re-check version for migration %d: %w", m.Version, err)
	}
	if m.Version <= cur {
		_ = tx.Rollback()
		return false, nil
	}

	if m.SQL != "" {
		if _, err := tx.ExecContext(context.Background(), ExecutableSQL(m)); err != nil {
			_ = tx.Rollback()
			if isBusyErr(err) {
				return false, err // retryable
			}
			return false, fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Description, err)
		}
	}
	if m.Apply != nil {
		if err := m.Apply(context.Background(), tx.Tx); err != nil {
			_ = tx.Rollback()
			if isBusyErr(err) {
				return false, err // retryable
			}
			return false, fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Description, err)
		}
	}

	if _, err := tx.ExecContext(context.Background(),
		fmt.Sprintf("INSERT INTO %s (version, description, identity) VALUES (?, ?, ?)", tableName),
		m.Version, m.Description, MigrationIdentity(m),
	); err != nil {
		_ = tx.Rollback()
		if isBusyErr(err) {
			return false, err // retryable
		}
		if isUniqueErr(err) {
			// Another applier recorded this version first: retry, re-check, and
			// read it as already-applied.
			return false, fmt.Errorf("%w: migration %d: %w", errLostRace, m.Version, err)
		}
		return false, fmt.Errorf("record migration %d: %w", m.Version, err)
	}

	if err := tx.Commit(); err != nil {
		if isBusyErr(err) {
			return false, err
		}
		return false, fmt.Errorf("commit migration %d: %w", m.Version, err)
	}
	return true, nil
}

// isUniqueErr reports a UNIQUE-constraint violation (both drivers include the
// standard SQLite message text).
func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// ensureIdentityColumn adds the identity column to a ledger created before
// identities existed.
func ensureIdentityColumn(db *DB, tableName string) error {
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'identity'`, tableName).Scan(&n); err != nil {
		return fmt.Errorf("read the columns of %s: %w", tableName, err)
	}
	if n > 0 {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE " + tableName + " ADD COLUMN identity TEXT NOT NULL DEFAULT ''"); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("add the identity column to %s: %w", tableName, err)
	}
	return nil
}

// checkAndAdopt compares the ledger with the code (CheckLedger) and records
// the code's identity on each row recorded before identities existed whose
// name CheckLedger accepted. The recorded name is never overwritten.
func checkAndAdopt(db *DB, tableName string, ordered []Migration) error {
	rows, err := db.QueryContext(context.Background(), "SELECT version, description, identity FROM "+tableName)
	if err != nil {
		return fmt.Errorf("read %s: %w", tableName, err)
	}
	var applied []AppliedMigration
	for rows.Next() {
		var a AppliedMigration
		if err := rows.Scan(&a.Version, &a.Description, &a.Identity); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read %s: %w", tableName, err)
		}
		applied = append(applied, a)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("read %s: %w", tableName, err)
	}
	adopt, err := CheckLedger(tableName, ordered, applied, shippedLedgers[tableName])
	if err != nil {
		return err
	}
	for _, m := range adopt {
		if _, err := db.Exec("UPDATE "+tableName+" SET identity = ? WHERE version = ? AND identity = ''",
			MigrationIdentity(m), m.Version); err != nil {
			return fmt.Errorf("adopt version %d of %s: %w", m.Version, tableName, err)
		}
	}
	return nil
}
