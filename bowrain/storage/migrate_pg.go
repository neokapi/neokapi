package storage

import (
	"context"
	"fmt"
	"regexp"
)

// validTableName ensures the namespace only contains safe characters for a table name.
var validTableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// migrationLockKey is the fixed pg_advisory_lock key that serializes schema
// migrations across concurrently booting instances (bowrain-server and
// bowrain-worker race on first boot; without the lock both pass the
// check-version step and one crashes applying the same migration). The value
// is arbitrary but must stay stable across releases.
const migrationLockKey int64 = 0x626F775F6D696772 // "bow_migr"

// MigratePostgres applies schema migrations to a PostgreSQL database using a
// namespaced migration tracking table. Each subsystem (store, auth, jobs, tm)
// should use a distinct namespace to avoid version collisions when sharing a DB.
func MigratePostgres(db *PgDB, migrations []Migration) error {
	return MigratePostgresNS(db, "schema_migrations", migrations)
}

// MigratePostgresNS applies schema migrations using a custom-named tracking
// table. The whole check-version-then-apply pass runs under a session-scoped
// Postgres advisory lock so concurrent instances migrating the same database
// serialize instead of racing.
func MigratePostgresNS(db *PgDB, tableName string, migrations []Migration) error {
	if !validTableName.MatchString(tableName) {
		return fmt.Errorf("invalid migration table name: %q", tableName)
	}

	ctx := context.Background()

	// pg_advisory_lock is session-scoped: it must be acquired, held, and
	// released on the SAME backend connection. database/sql pools
	// connections, so pin one for the entire migration pass.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		// Best-effort: closing the connection also releases the lock.
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	return runMigrations(conn, tableName, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, tableName),
		fmt.Sprintf("INSERT INTO %s (version, description) VALUES ($1, $2)", tableName),
		migrations)
}
