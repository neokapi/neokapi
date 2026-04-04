package storage_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_InMemory(t *testing.T) {
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Verify pragmas were applied.
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	// In-memory databases use "memory" journal mode regardless.
	assert.Contains(t, []string{"wal", "memory"}, journalMode)

	// Verify foreign keys are on.
	var fk int
	err = db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	require.NoError(t, err)
	assert.Equal(t, 1, fk)
}

func TestMigrate(t *testing.T) {
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	migrations := []storage.Migration{
		{
			Version:     1,
			Description: "create users table",
			SQL:         "CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL)",
		},
		{
			Version:     2,
			Description: "add email column",
			SQL:         "ALTER TABLE users ADD COLUMN email TEXT",
		},
	}

	// Apply all migrations.
	err = storage.Migrate(db, "test_migrations", migrations)
	require.NoError(t, err)

	// Insert a row to verify schema.
	_, err = db.Exec("INSERT INTO users (id, name, email) VALUES ('u1', 'Alice', 'alice@example.com')")
	require.NoError(t, err)

	// Re-running migrations should be idempotent.
	err = storage.Migrate(db, "test_migrations", migrations)
	require.NoError(t, err)

	// Verify migration was recorded.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_migrations").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestMigrate_Incremental(t *testing.T) {
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Apply first migration.
	migrations1 := []storage.Migration{
		{Version: 1, Description: "v1", SQL: "CREATE TABLE t1 (id TEXT PRIMARY KEY)"},
	}
	err = storage.Migrate(db, "test_migrations", migrations1)
	require.NoError(t, err)

	// Add a second migration.
	migrations2 := []storage.Migration{
		{Version: 1, Description: "v1", SQL: "CREATE TABLE t1 (id TEXT PRIMARY KEY)"},
		{Version: 2, Description: "v2", SQL: "CREATE TABLE t2 (id TEXT PRIMARY KEY)"},
	}
	err = storage.Migrate(db, "test_migrations", migrations2)
	require.NoError(t, err)

	// Both tables should exist.
	_, err = db.Exec("INSERT INTO t1 (id) VALUES ('a')")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t2 (id) VALUES ('b')")
	require.NoError(t, err)
}

func TestMigrate_InvalidTableName(t *testing.T) {
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = storage.Migrate(db, "Robert'; DROP TABLE--", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid migration table name")
}

func TestMigrate_NamespacedIsolation(t *testing.T) {
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	m1 := []storage.Migration{
		{Version: 1, Description: "tm table", SQL: "CREATE TABLE tm (id TEXT PRIMARY KEY)"},
	}
	m2 := []storage.Migration{
		{Version: 1, Description: "tb table", SQL: "CREATE TABLE tb (id TEXT PRIMARY KEY)"},
	}

	// Two subsystems can independently track version 1.
	require.NoError(t, storage.Migrate(db, "sievepen_migrations", m1))
	require.NoError(t, storage.Migrate(db, "termbase_migrations", m2))

	// Both tables should exist.
	_, err = db.Exec("INSERT INTO tm (id) VALUES ('a')")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO tb (id) VALUES ('b')")
	require.NoError(t, err)
}
