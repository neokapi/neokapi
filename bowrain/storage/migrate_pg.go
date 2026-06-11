package storage

import (
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
	return runMigrations(db, tableName, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, tableName),
		fmt.Sprintf("INSERT INTO %s (version, description) VALUES ($1, $2)", tableName),
		migrations)
}
