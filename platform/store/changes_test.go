package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChanges_SourceAdded(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// Store a new block → should log "source_added".
	blocks := []*model.Block{model.NewBlock("b1", "Hello")}
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", blocks))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, cs.Changes, 1)
	assert.Equal(t, "b1", cs.Changes[0].BlockID)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
	assert.NotEmpty(t, cs.Changes[0].ContentHash)
	assert.False(t, cs.HasMore)
	assert.Greater(t, cs.NewCursor, int64(0))
}

func TestGetChanges_SourceModified(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// Store initial block.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello")}))

	// Modify block content.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello World")}))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, cs.Changes, 2)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
	assert.Equal(t, "source_modified", cs.Changes[1].ChangeType)
}

func TestGetChanges_SourceRemoved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello")}))
	require.NoError(t, s.DeleteBlock(ctx, p.ID, "", "b1"))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, cs.Changes, 2)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
	assert.Equal(t, "source_removed", cs.Changes[1].ChangeType)
	assert.Empty(t, cs.Changes[1].ContentHash)
}

func TestGetChanges_UnchangedBlockNotLogged(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	b := model.NewBlock("b1", "Hello")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

	// Store the same block again (same content) → no "source_modified" entry.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 1) // Only the initial "source_added"
}

func TestGetChanges_Pagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// Create 5 blocks.
	for i := 0; i < 5; i++ {
		b := model.NewBlock("b"+string(rune('0'+i)), "text")
		require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))
	}

	// Fetch first 3.
	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 3)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 3)
	assert.True(t, cs.HasMore)

	// Fetch remaining from cursor.
	cs2, err := s.GetChanges(ctx, p.ID, "", cs.NewCursor, nil, 3)
	require.NoError(t, err)
	assert.Len(t, cs2.Changes, 2)
	assert.False(t, cs2.HasMore)
}

func TestGetChanges_CursorReturnsEmptyWhenNoChanges(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	assert.Empty(t, cs.Changes)
	assert.Equal(t, int64(0), cs.NewCursor)
	assert.False(t, cs.HasMore)
}

func TestLatestCursor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// No changes yet.
	cursor, err := s.LatestCursor(ctx, p.ID, "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), cursor)

	// Add blocks.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello")}))

	cursor, err = s.LatestCursor(ctx, p.ID, "")
	require.NoError(t, err)
	assert.Greater(t, cursor, int64(0))
}

func TestCompactChangeLog(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// Create and modify a block several times.
	for i := 0; i < 5; i++ {
		b := model.NewBlock("b1", "version "+string(rune('0'+i)))
		require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))
	}

	// Verify we have 5 entries (1 add + 4 modify).
	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 5)

	// Compact with 0 retention → keeps only latest per block.
	deleted, err := s.CompactChangeLog(ctx, p.ID, "", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(4), deleted)

	// Verify only 1 entry remains.
	cs, err = s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 1)
}

func TestGetChanges_MultiLocaleFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// Store a block (creates a source_added entry with locale=NULL).
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello")}))

	// Insert target change entries for multiple locales directly.
	now := time.Now().UTC().Format(time.RFC3339)
	insertTarget := func(locale string) {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO change_log (project_id, block_id, change_type, locale, content_hash, logged_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			p.ID, "b1", "target_added", sql.NullString{String: locale, Valid: true}, "hash-"+locale, now)
		require.NoError(t, err)
	}
	insertTarget("fr-FR")
	insertTarget("de-DE")
	insertTarget("ja-JP")

	// All changes (no locale filter): 1 source + 3 target = 4.
	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 4)

	// Filter by single locale: source (NULL) + fr-FR = 2.
	cs, err = s.GetChanges(ctx, p.ID, "", 0, []string{"fr-FR"}, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 2)

	// Filter by two locales: source (NULL) + fr-FR + de-DE = 3.
	cs, err = s.GetChanges(ctx, p.ID, "", 0, []string{"fr-FR", "de-DE"}, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 3)

	// Filter by all three locales: source (NULL) + all 3 = 4.
	cs, err = s.GetChanges(ctx, p.ID, "", 0, []string{"fr-FR", "de-DE", "ja-JP"}, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 4)

	// Filter by non-existent locale: only source (NULL) = 1.
	cs, err = s.GetChanges(ctx, p.ID, "", 0, []string{"ko-KR"}, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 1)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
}
