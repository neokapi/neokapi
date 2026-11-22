package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
)

type coverageStore interface {
	CreateProject(ctx context.Context, p *platstore.Project) error
	CreateCollection(ctx context.Context, c *platstore.Collection) error
	StoreItem(ctx context.Context, projectID, stream string, item *platstore.Item) error
	StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error
	CountCollectionCoverage(ctx context.Context, projectID, stream string, locales []string) ([]platstore.CollectionCoverage, error)
}

// Coverage counts translatable blocks per collection and the established ones
// per locale; untranslatable blocks and other locales stay out.
func runCollectionCoverageCases(t *testing.T, s coverageStore) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, s.CreateProject(ctx, &platstore.Project{
		ID: "p1", Name: "P", TargetLanguages: []model.LocaleID{"nb", "sv"},
	}))
	block := func(id string, status model.TargetStatus, translatable bool) *model.Block {
		b := model.NewBlock(id, "source "+id)
		b.Translatable = translatable
		if status != "" {
			b.SetTargetText("nb", "mål "+id)
			b.StampTargetProvenance("nb", status, model.Origin{Kind: model.OriginHuman})
		}
		return b
	}
	docs := &platstore.Collection{ID: "c-docs", ProjectID: "p1", Name: "docs"}
	require.NoError(t, s.CreateCollection(ctx, docs))
	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "guide.md", []*model.Block{
		block("d1", model.TargetStatusEstablished, true),
		block("d2", model.TargetStatusDraft, true),
		block("d3", "", true),
		block("d4", model.TargetStatusEstablished, false),
	}))
	require.NoError(t, s.StoreItem(ctx, "p1", "main", &platstore.Item{Name: "guide.md", CollectionID: docs.ID}))
	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "loose.md", []*model.Block{block("u1", "", true)}))

	got, err := s.CountCollectionCoverage(ctx, "p1", "", []string{"nb", "sv"})
	require.NoError(t, err)
	byID := map[string]platstore.CollectionCoverage{}
	for _, c := range got {
		byID[c.CollectionID] = c
	}
	require.Len(t, byID, 2)
	assert.Equal(t, 3, byID["c-docs"].Blocks, "the untranslatable block is not counted")
	assert.Equal(t, map[string]int{"nb": 1}, byID["c-docs"].Established)
	assert.Equal(t, 1, byID[""].Blocks, "a block of an item with no row is ungrouped")
	assert.Empty(t, byID[""].Established)
}

func TestCountCollectionCoverage_SQLite(t *testing.T) {
	s, err := sqlitestore.NewSQLiteStore(t.TempDir() + "/store.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	runCollectionCoverageCases(t, s)
}

func TestCountCollectionCoverage_Postgres(t *testing.T) {
	db := pgtest.NewTestDB(t)
	s, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	runCollectionCoverageCases(t, s)
}
