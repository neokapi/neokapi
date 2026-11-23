package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	corestorage "github.com/neokapi/neokapi/core/storage"
)

// The collision that reached production in kind, on the deploy that turns the
// check on: a database recorded version 2 as one migration before identities
// existed, and the code arrives with another migration numbered 2. The first
// start refuses, naming both; it adopts nothing, keeps the recorded name and
// applies nothing.
func TestMigratePostgres_RefusesAVersionRecordedUnderAnotherName(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()
	const table = "identity_test_schema_migrations"

	// A ledger as a released server left it: no identity column.
	_, err := db.ExecContext(ctx, `CREATE TABLE `+table+` (version INTEGER PRIMARY KEY, description TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE identity_test_a (x INTEGER); CREATE TABLE identity_test_b (x INTEGER)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO `+table+` (version, description) VALUES (1, 'the first'), (2, 'context session holds keyed by the item''s parts')`)
	require.NoError(t, err)

	first := storage.Migration{Version: 1, Description: "the first", SQL: `CREATE TABLE IF NOT EXISTS identity_test_a (x INTEGER)`}
	agentHost := storage.Migration{Version: 2, Description: "the agent host a review decision was recorded from", SQL: `ALTER TABLE identity_test_b ADD COLUMN y TEXT`}
	err = storage.MigratePostgresNS(db, table, []storage.Migration{first, agentHost})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `version 2 is recorded as "context session holds keyed by the item's parts" and the code's version 2 is "the agent host a review decision was recorded from"`)

	var desc, identity string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT description, identity FROM `+table+` WHERE version = 2`).Scan(&desc, &identity))
	assert.Equal(t, "context session holds keyed by the item's parts", desc, "the recorded name is kept")
	assert.Empty(t, identity, "nothing is adopted")
	var n int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'identity_test_b' AND column_name = 'y'`).Scan(&n))
	assert.Zero(t, n, "nothing is applied")

	// The code that matches the ledger starts, adopting both rows under
	// their recorded names.
	holds := storage.Migration{Version: 2, Description: "context session holds keyed by the item's parts", SQL: `CREATE TABLE IF NOT EXISTS identity_test_b (x INTEGER)`}
	require.NoError(t, storage.MigratePostgresNS(db, table, []storage.Migration{first, holds}))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT identity FROM `+table+` WHERE version = 2`).Scan(&identity))
	assert.Equal(t, corestorage.MigrationIdentity(holds), identity)

	err = storage.MigratePostgresNS(db, table, []storage.Migration{first, holds, {Version: 2, Description: "a second 2"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `version 2 is issued twice`)
}

// The server's SQLite ledger keeps identities the same way.
func TestMigrateSQLite_RefusesAShippedMigrationEdited(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "s.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	m := storage.Migration{Version: 1, Description: "create a", SQL: `CREATE TABLE a (x INTEGER)`}
	require.NoError(t, storage.MigrateContextNS(t.Context(), db, "test_migrations", []storage.Migration{m}))
	reworded := m
	reworded.SQL = "-- a comment someone reworded\nCREATE TABLE a (x INTEGER)"
	require.NoError(t, storage.MigrateContextNS(t.Context(), db, "test_migrations", []storage.Migration{reworded}), "a comment is not what executes")
	edited := m
	edited.SQL = `CREATE TABLE a (x TEXT)`
	err = storage.MigrateContextNS(t.Context(), db, "test_migrations", []storage.Migration{edited})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a shipped migration was edited")
}
