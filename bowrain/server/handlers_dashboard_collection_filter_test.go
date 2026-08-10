package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedDashboardCollections adds two collections to a project seeded by
// seedDashboardProject, moving two of its three items into them and leaving one
// where it was:
//
//	a.json → "col-app"   (5 words)
//	b.json → "col-docs"  (3 words)
//	c.json → no collection (1 word, fully translated)
func seedDashboardCollections(t *testing.T, srv *Server, pid string) {
	t.Helper()
	ctx := t.Context()
	cs := srv.ContentStore

	for _, c := range []struct{ id, name string }{{"col-app", "App"}, {"col-docs", "Docs"}} {
		require.NoError(t, cs.CreateCollection(ctx, &platstore.Collection{
			ID:        c.id,
			ProjectID: pid,
			Name:      c.name,
			Kind:      platstore.CollectionUploaded,
			Stream:    "main",
		}))
	}

	move := func(item, collectionID, text string) {
		b := &model.Block{ID: item + "-b1", Translatable: true}
		b.SetSourceText(text)
		require.NoError(t, cs.StoreItem(ctx, pid, "main", &platstore.Item{
			ProjectID: pid, Name: item, Format: "json", CollectionID: collectionID,
		}))
		require.NoError(t, cs.StoreBlocksForItem(ctx, pid, "main", item, []*model.Block{b}))
	}
	move("a.json", "col-app", "one two three four five")
	move("b.json", "col-docs", "one two three")
}

// TestTranslationDashboardCollectionFilter asserts that a drill-down into one
// collection is answered by the server rather than by the client filtering a
// page: item_stats holds that collection's items, item_total counts them, and
// the project-wide rollups it is read alongside stay complete.
func TestTranslationDashboardCollectionFilter(t *testing.T) {
	srv, token := newTestServer(t)
	pid := seedDashboardProject(t, srv, token)
	seedDashboardCollections(t, srv, pid)

	// Unfiltered: every item, and a total that counts them all.
	full := getDashboard(t, srv, token, pid, "")
	assert.Equal(t, []string{"a.json", "b.json", "c.json"}, itemNames(full))
	assert.Equal(t, 3, full.ItemTotal)

	// One collection: its items, and a total that counts only those.
	app := getDashboard(t, srv, token, pid, "?collection=col-app")
	assert.Equal(t, []string{"a.json"}, itemNames(app))
	assert.Equal(t, 1, app.ItemTotal, "item_total follows the filter")
	assert.Equal(t, 9, app.TotalSourceWords, "project totals stay complete under a filter")
	require.Len(t, app.LocaleStats, 1, "locale totals stay complete under a filter")
	assert.Len(t, app.CollectionStats, 3, "rollups stay complete under a filter")

	// The collection-less items are their own scope: no collection id can name
	// them, because that bucket's id is the empty string.
	loose := getDashboard(t, srv, token, pid, "?ungrouped=1")
	assert.Equal(t, []string{"c.json"}, itemNames(loose))
	assert.Equal(t, 1, loose.ItemTotal)

	// An empty `collection` is not the ungrouped bucket — it is no filter.
	assert.Equal(t, 3, getDashboard(t, srv, token, pid, "?collection=").ItemTotal)

	// A collection holding nothing yields an empty page, not an error.
	empty := getDashboard(t, srv, token, pid, "?collection=col-none")
	assert.Empty(t, empty.ItemStats)
	assert.Equal(t, 0, empty.ItemTotal)

	// Filter composes with sort and paging, and the total counts the filtered
	// list rather than the page.
	docs := getDashboard(t, srv, token, pid, "?collection=col-docs&limit=1&sort=words&dir=desc")
	assert.Equal(t, []string{"b.json"}, itemNames(docs))
	assert.Equal(t, 1, docs.ItemTotal)

	// Filtering must not mutate the cached full result.
	full = getDashboard(t, srv, token, pid, "")
	assert.Equal(t, []string{"a.json", "b.json", "c.json"}, itemNames(full))
	assert.Equal(t, 3, full.ItemTotal)

	// Asking for one collection and the collection-less bucket at once is a
	// contradiction, so it is rejected rather than resolved.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+pid+"/dashboard/main?collection=col-app&ungrouped=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
