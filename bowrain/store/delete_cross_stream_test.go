package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storeItemBlock(t *testing.T, s *PostgresStore, projectID, stream, itemName, sourceID, source string, targets map[model.LocaleID]string) string {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, s.StoreItem(ctx, projectID, stream, &platstore.Item{Name: itemName, Format: "json"}))
	b := model.NewBlock(sourceID, source)
	for locale, text := range targets {
		b.SetTargetText(locale, text)
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, projectID, stream, itemName, []*model.Block{b}))
	rows, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: projectID, Stream: stream, ItemName: itemName, Limit: 100})
	require.NoError(t, err)
	for _, sb := range rows {
		if sb.SourceID == sourceID {
			return sb.Block.ID
		}
	}
	return ""
}

// TestDeleteItem_BranchDeleteKeepsMainBlocks pins the cross-stream data-loss
// fix on Postgres: streams share block rows, so removing a file on a branch
// must not destroy main's source blocks or targets.
func TestDeleteItem_BranchDeleteKeepsMainBlocks(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	id := storeItemBlock(t, s, p.ID, "main", "file.json", "greeting", "Hello",
		map[model.LocaleID]string{model.LocaleFrench: "Bonjour"})
	require.NotEmpty(t, id)

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{ProjectID: p.ID, Name: "feature", Parent: "main"}))
	require.NoError(t, s.DeleteItem(ctx, p.ID, "feature", "file.json"))

	got, err := s.GetBlock(ctx, p.ID, "main", id)
	require.NoError(t, err, "main's block must survive a branch-side delete")
	assert.Equal(t, "Hello", got.Block.SourceText())
	assert.Equal(t, "Bonjour", got.Block.TargetText(model.LocaleFrench),
		"main's approved target must survive a branch-side delete")

	require.NoError(t, s.DeleteItem(ctx, p.ID, "main", "file.json"))
	_, err = s.GetBlock(ctx, p.ID, "main", id)
	require.Error(t, err, "the last stream's delete reclaims the shared block")
}

// TestDeleteItem_NoTargetResurrection is the finding-2 guard on Postgres.
func TestDeleteItem_NoTargetResurrection(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	storeItemBlock(t, s, p.ID, "main", "file.json", "greeting", "Hello",
		map[model.LocaleID]string{model.LocaleFrench: "Bonjour"})
	require.NoError(t, s.DeleteItem(ctx, p.ID, "main", "file.json"))

	id := storeItemBlock(t, s, p.ID, "main", "file.json", "greeting", "Goodbye", nil)
	require.NotEmpty(t, id)

	got, err := s.GetBlock(ctx, p.ID, "main", id)
	require.NoError(t, err)
	assert.Equal(t, "Goodbye", got.Block.SourceText())
	assert.Empty(t, got.Block.TargetText(model.LocaleFrench),
		"a stale target must not resurrect onto new source text")
}
