package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedFilterProject builds a project holding two collections and one item that
// belongs to neither:
//
//	a.json → "col-app"   (5 words)
//	b.json → "col-docs"  (3 words)
//	c.json → no collection at all (1 word)
//
// Built on the content store rather than through POST /projects, because that
// handler calls EnsureDefaultCollection and StoreItem then resolves an empty
// collection id to that default (bowrain/store/{postgres.go,sqlitestore/
// sqlite.go}). An API-created project therefore has no ungrouped bucket at all,
// and the collection-less scope needs one to answer for.
func seedFilterProject(t *testing.T, cs platstore.ContentStore, workspaceID string) string {
	t.Helper()
	ctx := t.Context()

	proj := &platstore.Project{
		Name:                  "Filter",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		WorkspaceID:           workspaceID,
		Properties:            map[string]string{},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, EnsureMainStream(ctx, cs, proj.ID))

	for _, c := range []struct{ id, name string }{{"col-app", "App"}, {"col-docs", "Docs"}} {
		require.NoError(t, cs.CreateCollection(ctx, &platstore.Collection{
			ID:        c.id,
			ProjectID: proj.ID,
			Name:      c.name,
			Kind:      platstore.CollectionUploaded,
			Stream:    "main",
		}))
	}

	seed := func(item, collectionID, text string) {
		b := &model.Block{ID: item + "-b1", Translatable: true}
		b.SetSourceText(text)
		require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
			ProjectID: proj.ID, Name: item, Format: "json", ItemType: "file",
			CollectionID: collectionID,
		}))
		require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", item, []*model.Block{b}))
	}
	seed("a.json", "col-app", "one two three four five")
	seed("b.json", "col-docs", "one two three")
	seed("c.json", "", "one")

	return proj.ID
}

func windowNames(stats *platstore.TranslationDashboardStats) []string {
	names := make([]string, len(stats.ItemStats))
	for i, it := range stats.ItemStats {
		names[i] = it.ItemName
	}
	return names
}

// TestDashboardItemWindow asserts what a drill-down into one collection
// returns: that collection's items, a total counting them rather than the
// project, and project-wide rollups left complete — all without the client
// filtering a page it was handed.
//
// Exercised against the store directly (sqlite needs no container), so the
// filtering, ordering and paging are verified rather than assumed. The
// endpoint test below covers the query parameters that reach this window.
func TestDashboardItemWindow(t *testing.T) {
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "filter.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	ctx := t.Context()
	pid := seedFilterProject(t, cs, "ws-1")
	proj, err := cs.GetProject(ctx, pid)
	require.NoError(t, err)
	stats, err := editorGetDashboardStats(ctx, cs, proj, "main")
	require.NoError(t, err)

	// The fixture's shape, before any window narrows it.
	require.Equal(t, 3, stats.ItemTotal)
	require.Len(t, stats.CollectionStats, 3)
	// Named, so a count alone can never pass for the wrong three: the two
	// collections by name, then the ungrouped bucket last.
	assert.Equal(t, "App", stats.CollectionStats[0].CollectionName)
	assert.Equal(t, "Docs", stats.CollectionStats[1].CollectionName)
	assert.True(t, stats.CollectionStats[2].Ungrouped,
		"the third bucket is the ungrouped one, not a default collection")

	// One collection: its items, and a total that counts only those.
	app := pageDashboardStats(stats, dashboardItemWindow{collectionID: "col-app"})
	assert.Equal(t, []string{"a.json"}, windowNames(app))
	assert.Equal(t, 1, app.ItemTotal, "item_total follows the filter")
	assert.Equal(t, 9, app.TotalSourceWords, "project totals stay complete under a filter")
	assert.Len(t, app.LocaleStats, 1, "locale totals stay complete under a filter")
	assert.Len(t, app.CollectionStats, 3, "rollups stay complete under a filter")

	// The collection-less items are their own scope: no collection id can name
	// them, because that bucket's id is the empty string.
	loose := pageDashboardStats(stats, dashboardItemWindow{ungrouped: true})
	assert.Equal(t, []string{"c.json"}, windowNames(loose))
	assert.Equal(t, 1, loose.ItemTotal)

	// An empty collection id is not the ungrouped bucket — it is no filter.
	unfiltered := pageDashboardStats(stats, dashboardItemWindow{})
	assert.Equal(t, 3, unfiltered.ItemTotal)
	assert.Len(t, unfiltered.ItemStats, 3)

	// A collection holding nothing yields an empty page, not an error.
	empty := pageDashboardStats(stats, dashboardItemWindow{collectionID: "col-none"})
	assert.Empty(t, empty.ItemStats)
	assert.Equal(t, 0, empty.ItemTotal)

	// Filter composes with sort and paging, and the total counts the filtered
	// list rather than the page it was sliced into.
	paged := pageDashboardStats(stats, dashboardItemWindow{
		collectionID: "col-docs", limit: 1, sortField: "words", dir: "desc",
	})
	assert.Equal(t, []string{"b.json"}, windowNames(paged))
	assert.Equal(t, 1, paged.ItemTotal)

	// The cached full result is the input to every window, so no window may
	// touch it.
	assert.Equal(t, []string{"a.json", "b.json", "c.json"}, windowNames(stats))
	assert.Equal(t, 3, stats.ItemTotal)
}

// TestTranslationDashboardCollectionFilterEndpoint asserts the query parameters
// that build the window: which collection, the collection-less bucket, and the
// contradiction of asking for both.
func TestTranslationDashboardCollectionFilterEndpoint(t *testing.T) {
	srv, token := newTestServer(t)
	pid := seedFilterProject(t, srv.ContentStore, "test-ws")

	assert.Equal(t, []string{"a.json"}, itemNames(getDashboard(t, srv, token, pid, "?collection=col-app")))
	assert.Equal(t, 1, getDashboard(t, srv, token, pid, "?collection=col-app").ItemTotal)
	assert.Equal(t, []string{"c.json"}, itemNames(getDashboard(t, srv, token, pid, "?ungrouped=1")))
	assert.Equal(t, 3, getDashboard(t, srv, token, pid, "?collection=").ItemTotal,
		"an empty collection parameter is no filter, not the ungrouped bucket")

	// Asking for one collection and the collection-less bucket at once is a
	// contradiction, so it is rejected rather than resolved.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+pid+"/dashboard/main?collection=col-app&ungrouped=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
