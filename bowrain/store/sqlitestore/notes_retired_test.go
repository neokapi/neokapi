package sqlitestore

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A desktop working copy built before notes became annotations on the block
// holds the block_notes table and its rows. Opening it with this version drops
// the table, rows and all; a working copy built since has none to drop.
func TestMigrations_DropTheRetiredBlockNotesTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.db")
	s, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	ctx := t.Context()

	tableExists := func(s *SQLiteStore) bool {
		var n int
		require.NoError(t, s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'block_notes'`).Scan(&n))
		return n > 0
	}
	require.False(t, tableExists(s), "a working copy built from the current baseline has no notes table")

	// The schema a working copy at version 25 carries: the notes table, and
	// nothing a later version adds.
	_, err = s.db.ExecContext(ctx, `DROP TABLE pre_reviews`)
	require.NoError(t, err)
	for _, stmt := range []string{
		`ALTER TABLE unit_decisions DROP COLUMN revision`,
		`ALTER TABLE unit_decisions DROP COLUMN basis`,
		`ALTER TABLE unit_decisions ADD COLUMN target_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE unit_decisions ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE blocks DROP COLUMN source_revision`,
		`ALTER TABLE block_history DROP COLUMN basis`,
		`ALTER TABLE block_history DROP COLUMN basis_from`,
	} {
		_, err = s.db.ExecContext(ctx, stmt)
		require.NoError(t, err, stmt)
	}
	_, err = s.db.ExecContext(ctx, `CREATE TABLE block_notes (
		id         TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		block_id   TEXT NOT NULL,
		author     TEXT NOT NULL DEFAULT '',
		text       TEXT NOT NULL,
		stream     TEXT NOT NULL DEFAULT 'main',
		created_at TEXT NOT NULL DEFAULT (datetime('now')))`)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO block_notes (id, project_id, block_id, author, text) VALUES ('n1', 'p1', 'b1', 'reviewer', 'the source reads oddly here')`)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version >= 26`)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	reopened, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	assert.False(t, tableExists(reopened), "opening the working copy drops the retired notes table")
}
