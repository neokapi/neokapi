package storage

import (
	"context"
	"fmt"
	"regexp"
)

// validTableName ensures the namespace only contains safe characters for a table name.
var validTableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// MigratePostgres applies schema migrations to a PostgreSQL database using a
// namespaced migration tracking table. Each subsystem (store, auth, jobs, tm)
// should use a distinct namespace to avoid version collisions when sharing a DB.
func MigratePostgres(db *PgDB, migrations []Migration) error {
	return MigratePostgresNS(db, "schema_migrations", migrations)
}

// MigratePostgresNS applies schema migrations using a custom-named tracking table.
func MigratePostgresNS(db *PgDB, tableName string, migrations []Migration) error {
	if !validTableName.MatchString(tableName) {
		return fmt.Errorf("invalid migration table name: %q", tableName)
	}

	ctx := context.Background()

	// Create migration tracking table.
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, tableName)); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Get current version.
	var currentVersion int
	err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM "+tableName).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	// Apply pending migrations.
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

		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf("INSERT INTO %s (version, description) VALUES ($1, $2)", tableName),
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
