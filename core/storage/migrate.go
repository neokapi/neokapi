package storage

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Migration represents a single schema migration step.
type Migration struct {
	Version     int
	Description string
	SQL         string
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

	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`, tableName)); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	var currentVersion int
	err := db.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(version), 0) FROM "+tableName).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	// Apply in ascending version order regardless of how the caller ordered the
	// slice, and reject duplicate versions, so correctness doesn't depend on
	// every call site hand-sorting its migrations.
	ordered := slices.Clone(migrations)
	slices.SortFunc(ordered, func(a, b Migration) int { return cmp.Compare(a.Version, b.Version) })
	for i := 1; i < len(ordered); i++ {
		if ordered[i].Version == ordered[i-1].Version {
			return fmt.Errorf("duplicate migration version %d in %q", ordered[i].Version, tableName)
		}
	}

	for _, m := range ordered {
		if m.Version <= currentVersion {
			continue
		}
		if err := applyMigration(db, tableName, m); err != nil {
			return err
		}
	}

	return nil
}

// errLostRace marks the one non-lock condition applyMigration retries: another
// applier committed this migration's version row between our in-transaction
// re-check and our own insert. Only the version-row INSERT raises it. A UNIQUE
// violation from a migration's *own* statements is a real fault — retrying it
// for the whole window would bury the actual error under a timeout — so the
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

	if _, err := tx.ExecContext(context.Background(), m.SQL); err != nil {
		_ = tx.Rollback()
		if isBusyErr(err) {
			return false, err // retryable
		}
		return false, fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Description, err)
	}

	if _, err := tx.ExecContext(context.Background(),
		fmt.Sprintf("INSERT INTO %s (version, description) VALUES (?, ?)", tableName),
		m.Version, m.Description,
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
