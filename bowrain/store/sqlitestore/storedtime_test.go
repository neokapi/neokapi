package sqlitestore

import (
	"testing"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A timestamp column that no longer parses is stored corruption. The read
// reports it, rather than returning a row whose time reads as "never".
func TestCorruptStoredTimestampIsReported(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	t.Run("project created_at", func(t *testing.T) {
		_, err := s.db.ExecContext(ctx, `UPDATE projects SET created_at='not a time' WHERE id=?`, p.ID)
		require.NoError(t, err)
		got, err := s.GetProject(ctx, p.ID)
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "project "+p.ID)
		assert.Contains(t, err.Error(), "parse created_at")
	})

	t.Run("project archived_at", func(t *testing.T) {
		_, err := s.db.ExecContext(ctx,
			`UPDATE projects SET created_at=?, archived_at='yesterday' WHERE id=?`,
			time.Now().UTC().Format(time.RFC3339), p.ID)
		require.NoError(t, err)
		_, err = s.GetProject(ctx, p.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse archived_at")

		_, err = s.db.ExecContext(ctx, `UPDATE projects SET archived_at=NULL WHERE id=?`, p.ID)
		require.NoError(t, err)
		got, err := s.GetProject(ctx, p.ID)
		require.NoError(t, err)
		assert.Nil(t, got.ArchivedAt)
	})

	t.Run("collection updated_at", func(t *testing.T) {
		c := &platstore.Collection{ProjectID: p.ID, Name: "docs"}
		require.NoError(t, s.CreateCollection(ctx, c))
		_, err := s.db.ExecContext(ctx, `UPDATE collections SET updated_at='' WHERE id=?`, c.ID)
		require.NoError(t, err)
		got, err := s.GetCollection(ctx, p.ID, c.ID)
		require.NoError(t, err, "an empty column is a never-set value, not corruption")
		assert.True(t, got.UpdatedAt.IsZero())

		_, err = s.db.ExecContext(ctx, `UPDATE collections SET updated_at='2026-13-45' WHERE id=?`, c.ID)
		require.NoError(t, err)
		_, err = s.GetCollection(ctx, p.ID, c.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "collection "+c.ID)
		assert.Contains(t, err.Error(), "parse updated_at")
	})
}

// A row that took the column's datetime('now') default carries SQLite's own
// layout. The read accepts it beside RFC3339 rows, and still reports a value
// that matches neither.
func TestBlockHistoryAcceptsLegacyTimestampLayout(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	insert := func(createdAt string) {
		t.Helper()
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO block_history (project_id, stream, block_id, locale, change_type, text, coded_text, origin, author, created_at)
			 VALUES (?, 'main', 'b1', 'fr', 'target_modified', 'Bonjour', '', 'human', 'reviewer', ?)`,
			p.ID, createdAt)
		require.NoError(t, err)
	}

	insert("2026-07-01 10:00:00")
	insert("2026-07-01T11:00:00Z")
	entries, err := s.GetBlockHistory(ctx, p.ID, "main", "b1", "fr", 0)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.True(t, time.Date(2026, 7, 1, 11, 0, 0, 0, time.UTC).Equal(entries[0].Timestamp))
	assert.True(t, time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).Equal(entries[1].Timestamp))

	insert("last tuesday")
	_, err = s.GetBlockHistory(ctx, p.ID, "main", "b1", "fr", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "block history entry")
	assert.Contains(t, err.Error(), "parse created_at")
}
