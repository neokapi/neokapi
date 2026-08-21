//go:build integration

package auth_test

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rename that reaches only the CREATE moves the column for a database built
// after it and leaves it alone in every database built before — while the
// queries move for both. That is not a hypothetical: it took every workspace
// query in production down with `column w.voice_profile_id does not exist`,
// because CREATE TABLE IF NOT EXISTS is a no-op on a table that already exists
// and the baseline was not re-applied besides.
//
// So the baseline is asserted against a database that predates the rename, not
// only against an empty one.
func TestBaselineRenamesWorkspaceVoiceColumn(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	// A database as it stood before the rename: the column under its former
	// name, and the ledger claiming a version at or above the old baseline so
	// nothing would replay on its own.
	_, err := db.ExecContext(ctx, `
		CREATE TABLE workspaces (
			id                     TEXT PRIMARY KEY,
			name                   TEXT NOT NULL,
			slug                   TEXT UNIQUE NOT NULL,
			brand_voice_profile_id TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO workspaces (id, name, slug, brand_voice_profile_id)
		VALUES ('ws1', 'Acme', 'acme', 'house-voice');`)
	require.NoError(t, err)

	require.NoError(t, storage.MigratePostgresNS(db, "auth_schema_migrations", auth.Migrations))

	var profileID string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT voice_profile_id FROM workspaces WHERE id = 'ws1'`).Scan(&profileID),
		"the baseline must rename the column an existing database still carries")
	assert.Equal(t, "house-voice", profileID, "the rename must carry the rows, not replace them")

	// Idempotent: the baseline is re-applied by design, and the guard must make
	// the second pass a no-op rather than an error.
	require.NoError(t, storage.MigratePostgresNS(db, "auth_schema_migrations", auth.Migrations))
}

// The same baseline against an empty database: the CREATE already names the
// column, so the rename must not fire and must not fail.
func TestBaselineOnAnEmptyDatabaseNamesTheColumnDirectly(t *testing.T) {
	db := pgtest.NewTestDB(t)

	require.NoError(t, storage.MigratePostgresNS(db, "auth_schema_migrations", auth.Migrations))

	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'workspaces' AND column_name = 'voice_profile_id'`).Scan(&n))
	assert.Equal(t, 1, n)

	require.NoError(t, db.QueryRowContext(t.Context(), `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'workspaces' AND column_name = 'brand_voice_profile_id'`).Scan(&n))
	assert.Zero(t, n, "the former name must not survive on a fresh database")
}
