package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
)

// A database built before notes became annotations on the block holds the
// block_notes table and its rows. Migrating it drops the table, rows and all;
// a database built since has none to drop.
func TestMigrations_DropTheRetiredBlockNotesTable(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	tableExists := func() bool {
		var exists bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass('public.block_notes') IS NOT NULL`).Scan(&exists))
		return exists
	}
	require.False(t, tableExists(), "a database built from the current baseline has no notes table")

	// The schema a database at version 37 carries: the notes table, and
	// nothing a later version adds.
	_, err := db.ExecContext(ctx, `DROP TABLE pre_reviews`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE block_notes (
		id         TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		block_id   TEXT NOT NULL,
		author     TEXT NOT NULL DEFAULT '',
		text       TEXT NOT NULL,
		stream     TEXT NOT NULL DEFAULT 'main',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO block_notes (id, project_id, block_id, author, text) VALUES ('n1', 'p1', 'b1', 'reviewer', 'the source reads oddly here')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM store_schema_migrations WHERE version >= 38`)
	require.NoError(t, err)

	require.NoError(t, storage.MigratePostgresNS(db, "store_schema_migrations", bstore.Migrations))
	assert.False(t, tableExists(), "migrating drops the retired notes table")
}
