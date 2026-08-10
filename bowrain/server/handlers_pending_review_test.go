package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// The pending-review endpoint's collection scope is a query parameter whose
// PRESENCE is the filter, not its value: `?collection=` selects the items in no
// collection, which is a scope of its own. A reviewer arriving from a
// collection card used to receive the project's whole queue and narrow it in
// the browser over a bounded slice, so a large collection showed fewer entries
// than its card counted, and the reported total was the project's.
func TestHandleListPendingReview_CollectionScope(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	initTestStores(t, srv)
	cs := srv.ContentStore
	ctx := t.Context()

	const pid = "p-pending-scope"
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: pid, Name: "Scope", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"},
	}))
	docs := &platstore.Collection{ID: "c-docs", ProjectID: pid, Name: "Docs"}
	require.NoError(t, cs.CreateCollection(ctx, docs))

	pending := func(id string) *model.Block {
		b := model.NewBlock(id, "source "+id)
		b.Translatable = true
		b.SetTargetText("fr", "brouillon "+id)
		b.StampTargetProvenance("fr", model.TargetStatusDraft, model.Origin{Kind: model.OriginAI})
		return b
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, pid, "main", "guide.md", []*model.Block{pending("g1")}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, pid, "main", "loose.md", []*model.Block{pending("l1")}))
	require.NoError(t, cs.StoreItem(ctx, pid, "main", &platstore.Item{Name: "guide.md", CollectionID: docs.ID}))
	require.NoError(t, cs.StoreItem(ctx, pid, "main", &platstore.Item{Name: "loose.md"}))

	cases := []struct {
		name  string
		query string
		items []string
	}{
		{"no collection parameter is the whole project", "", []string{"guide.md", "loose.md"}},
		{"a named collection is that collection's queue", "?collection=c-docs", []string{"guide.md"}},
		{"an empty value is the ungrouped bucket", "?collection=", []string{"loose.md"}},
		{"an unknown collection is empty, not everything", "?collection=c-nope", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := editorCall(t, srv, http.MethodGet, pid,
				"/api/v1/acme/"+pid+"/pending-review/main"+tc.query, "", srv.HandleListPendingReview)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			page := decodeJSON[pendingReviewResponse](t, rec)

			names := make([]string, 0, len(page.Entries))
			for _, e := range page.Entries {
				names = append(names, e.ItemName)
			}
			assert.ElementsMatch(t, tc.items, names)
			assert.Equal(t, len(tc.items), page.Total,
				"the total describes the scoped queue, so a caller paging a collection is never told the project's size")
		})
	}
}

// Every entry names the collection its item belongs to, from the same join the
// filter tests — so a queue narrowed to a collection and a queue grouped by
// collection cannot disagree about where a row belongs.
func TestHandleListPendingReview_EntriesCarryTheirCollection(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	initTestStores(t, srv)
	cs := srv.ContentStore
	ctx := t.Context()

	const pid = "p-pending-collection-id"
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: pid, Name: "Carry", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"},
	}))
	site := &platstore.Collection{ID: "c-site", ProjectID: pid, Name: "Site"}
	require.NoError(t, cs.CreateCollection(ctx, site))

	b := model.NewBlock("s1", "Hello")
	b.Translatable = true
	b.SetTargetText("fr", "Bonjour")
	b.StampTargetProvenance("fr", model.TargetStatusTranslated, model.Origin{Kind: model.OriginAI})
	require.NoError(t, cs.StoreBlocksForItem(ctx, pid, "main", "index.md", []*model.Block{b}))
	require.NoError(t, cs.StoreItem(ctx, pid, "main", &platstore.Item{Name: "index.md", CollectionID: site.ID}))

	rec, err := editorCall(t, srv, http.MethodGet, pid,
		"/api/v1/acme/"+pid+"/pending-review/main", "", srv.HandleListPendingReview)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	page := decodeJSON[pendingReviewResponse](t, rec)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, "c-site", page.Entries[0].CollectionID)
	require.NotNil(t, page.Entries[0].Block, "the entry still hydrates its block")
}
