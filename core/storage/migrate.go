package storage

import (
	"fmt"
	"regexp"
)

// Migration represents a single schema migration step.
type Migration struct {
	Version     int
	Description string
	SQL         string
}

// validTableName ensures the namespace only contains safe characters for a table name.
var validTableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Migrate applies schema migrations to the database using a namespaced migration
// tracking table. Each subsystem (sievepen, termbase) should use a distinct
// namespace to avoid version collisions when sharing a database file.
func Migrate(db *DB, tableName string, migrations []Migration) error {
	if !validTableName.MatchString(tableName) {
		return fmt.Errorf("invalid migration table name: %q", tableName)
	}

	//nolint:noctx // startup migration
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
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM "+tableName).Scan(&currentVersion) //nolint:noctx // startup migration
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		tx, err := db.Begin() //nolint:noctx // startup migration
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(m.SQL); err != nil { //nolint:noctx // startup migration
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Description, err)
		}

		if _, err := tx.Exec( //nolint:noctx // startup migration
			fmt.Sprintf("INSERT INTO %s (version, description) VALUES (?, ?)", tableName),
			m.Version, m.Description,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}

	return nil
}
