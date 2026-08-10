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
		// An EMPTY status is below reviewed and therefore pending — the
		// review-completion gate counts it, so the queue must show it (an
		// IN ('draft','translated') spelling once hid it forever).
		mk("b6", "", "tomt", model.TargetStatusNew, true),
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "a.md", blocks[:3]))
	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "b.md", blocks[3:]))

	refs, total, err := s.ListPendingReview(ctx, platstore.PendingReviewQuery{
		ProjectID: "p1", Stream: "main", Locales: []string{"nb"}, Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, refs, 3)
	// The store assigns its own block ids; the item and locale are the
	// stable coordinates to assert on.
	for _, r := range refs[:2] {
		assert.Equal(t, "a.md", r.ItemName)
		assert.Equal(t, "nb", r.Locale)
		assert.NotEmpty(t, r.BlockID)
	}

	// Pagination holds the same order.
	page2, total2, err := s.ListPendingReview(ctx, platstore.PendingReviewQuery{
		ProjectID: "p1", Stream: "main", Limit: 1, Offset: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, total2)
	require.Len(t, page2, 1)
}

// pendingReviewStore is the slice of ContentStore the collection cases
// exercise, so the same scenario runs against both engines: the SQL that pages
// a scoped queue and the SQL that counts it must agree in each, and with each
// other.
type pendingReviewStore interface {
	CreateProject(ctx context.Context, p *platstore.Project) error
	CreateCollection(ctx context.Context, c *platstore.Collection) error
	StoreItem(ctx context.Context, projectID, stream string, item *platstore.Item) error
	StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error
	ListPendingReview(ctx context.Context, q platstore.PendingReviewQuery) ([]platstore.PendingReviewRef, int, error)
}

// runCollectionScopeCases pins the server-side collection filter. A
// collection-scoped review session used to receive the project's whole queue
// and narrow it in the browser over a bounded slice, so a collection larger
// than the slice showed fewer entries than its own card counted — and the total
// it reported was the project's, not the collection's.
func runCollectionScopeCases(t *testing.T, s pendingReviewStore) {
	t.Helper()
	ctx := t.Context()

	require.NoError(t, s.CreateProject(ctx, &platstore.Project{
		ID: "p1", Name: "P", TargetLanguages: []model.LocaleID{"nb"},
	}))

	pending := func(id string) *model.Block {
		b := model.NewBlock(id, "source "+id)
		b.Translatable = true
		b.SetTargetText("nb", "utkast "+id)
		b.StampTargetProvenance("nb", model.TargetStatusDraft, model.Origin{Kind: model.OriginAI})
		return b
	}
	// Two collections, an item explicitly in neither, and an item with no row at
	// all — the two ways a block reaches the ungrouped bucket. Both must land
	// there: the item row is the only thing that knows a block's collection, and
	// a queue that dropped the rowless block would be worse than one that files
	// it as ungrouped.
	docs := &platstore.Collection{ID: "c-docs", ProjectID: "p1", Name: "Docs"}
	require.NoError(t, s.CreateCollection(ctx, docs))
	site := &platstore.Collection{ID: "c-site", ProjectID: "p1", Name: "Site"}
	require.NoError(t, s.CreateCollection(ctx, site))

	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "guide.md", []*model.Block{pending("d1"), pending("d2")}))
	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "index.md", []*model.Block{pending("s1")}))
	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "loose.md", []*model.Block{pending("u1")}))
	require.NoError(t, s.StoreBlocksForItem(ctx, "p1", "main", "rowless.md", []*model.Block{pending("u2")}))
	require.NoError(t, s.StoreItem(ctx, "p1", "main", &platstore.Item{Name: "guide.md", CollectionID: docs.ID}))
	require.NoError(t, s.StoreItem(ctx, "p1", "main", &platstore.Item{Name: "index.md", CollectionID: site.ID}))
	require.NoError(t, s.StoreItem(ctx, "p1", "main", &platstore.Item{Name: "loose.md"}))

	scope := func(id *string) ([]platstore.PendingReviewRef, int) {
		t.Helper()
		refs, total, err := s.ListPendingReview(ctx, platstore.PendingReviewQuery{
			ProjectID: "p1", Stream: "main", CollectionID: id, Limit: 100,
		})
		require.NoError(t, err)
		return refs, total
	}
	names := func(refs []platstore.PendingReviewRef) []string {
		out := make([]string, 0, len(refs))
		for _, r := range refs {
			out = append(out, r.ItemName)
		}
		return out
	}

	all, total := scope(nil)
	assert.Equal(t, 5, total, "a nil scope imposes no constraint")
	assert.ElementsMatch(t,
		[]string{"guide.md", "guide.md", "index.md", "loose.md", "rowless.md"}, names(all))

	// Every entry carries the collection the filter matches on, so grouping and
	// filtering rest on the same evidence.
	byItem := map[string]string{}
	for _, r := range all {
		byItem[r.ItemName] = r.CollectionID
	}
	assert.Equal(t, docs.ID, byItem["guide.md"])
	assert.Equal(t, site.ID, byItem["index.md"])
	assert.Empty(t, byItem["loose.md"])
	assert.Empty(t, byItem["rowless.md"])

	docsRefs, docsTotal := scope(&docs.ID)
	assert.Equal(t, 2, docsTotal, "the total is the scoped queue's, not the project's")
	assert.ElementsMatch(t, []string{"guide.md", "guide.md"}, names(docsRefs))

	siteRefs, siteTotal := scope(&site.ID)
	assert.Equal(t, 1, siteTotal)
	assert.ElementsMatch(t, []string{"index.md"}, names(siteRefs))

	// The empty id is the ungrouped bucket — a scope of its own, distinct from
	// the absence of one.
	ungrouped := ""
	looseRefs, looseTotal := scope(&ungrouped)
	assert.Equal(t, 2, looseTotal)
	assert.ElementsMatch(t, []string{"loose.md", "rowless.md"}, names(looseRefs))

	// An unknown collection is empty, not the whole project.
	unknown := "c-nope"
	_, unknownTotal := scope(&unknown)
	assert.Zero(t, unknownTotal)

	// Paging inside a scope pages that scope: page 1 of the Docs queue is one
	// of its two entries, and its total still describes the collection.
	page, pageTotal, err := s.ListPendingReview(ctx, platstore.PendingReviewQuery{
		ProjectID: "p1", Stream: "main", CollectionID: &docs.ID, Limit: 1, Offset: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, pageTotal)
	require.Len(t, page, 1)
	assert.Equal(t, "guide.md", page[0].ItemName)
}

func TestListPendingReview_CollectionScope_SQLite(t *testing.T) {
	s, err := sqlitestore.NewSQLiteStore(t.TempDir() + "/store.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	runCollectionScopeCases(t, s)
}

func TestListPendingReview_CollectionScope_Postgres(t *testing.T) {
	db := pgtest.NewTestDB(t)
	s, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	runCollectionScopeCases(t, s)
}
