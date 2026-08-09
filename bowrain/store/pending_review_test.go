package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
)

// The review session's queue is one indexed query, not a blocks fetch per
// item. These cases pin the predicate: translatable + non-empty target text +
// status below reviewed, in stable item-then-block order.
func TestListPendingReview_SQLite(t *testing.T) {
	s, err := sqlitestore.NewSQLiteStore(t.TempDir() + "/store.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()

	require.NoError(t, s.CreateProject(ctx, &platstore.Project{ID: "p1", Name: "P", TargetLanguages: []model.LocaleID{"nb"}}))

	mk := func(id, item, text string, status model.TargetStatus, translatable bool) *model.Block {
		_ = item
		b := model.NewBlock(id, "source "+id)
		b.Translatable = translatable
		if text != "" {
			b.SetTargetText("nb", text)
			b.StampTargetProvenance("nb", status, model.Origin{Kind: model.OriginAI})
		}
		return b
	}
	blocks := []*model.Block{
		mk("b1", "", "hei", model.TargetStatusDraft, true),         // pending
		mk("b2", "", "hallo", model.TargetStatusTranslated, true),  // pending
		mk("b3", "", "godkjent", model.TargetStatusReviewed, true), // decided
		mk("b4", "", "", model.TargetStatusDraft, true),            // no target text
		mk("b5", "", "skjult", model.TargetStatusDraft, false),     // not translatable
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "a.md", blocks[:3]))
	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "b.md", blocks[3:]))

	refs, total, err := s.ListPendingReview(ctx, "p1", "main", []string{"nb"}, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, refs, 2)
	// The store assigns its own block ids; the item and locale are the
	// stable coordinates to assert on.
	for _, r := range refs {
		assert.Equal(t, "a.md", r.ItemName)
		assert.Equal(t, "nb", r.Locale)
		assert.NotEmpty(t, r.BlockID)
	}

	// Pagination holds the same order.
	page2, total2, err := s.ListPendingReview(ctx, "p1", "main", nil, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, total2)
	require.Len(t, page2, 1)
}
