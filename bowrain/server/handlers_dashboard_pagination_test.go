package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedDashboardProject creates a project (en → fr) with three items of
// distinct word counts and completion so every server-side sort column is
// distinguishable: c.json (1 word, fully translated), a.json (5 words,
// untranslated), b.json (3 words, untranslated).
func seedDashboardProject(t *testing.T, srv *Server, token string) string {
	t.Helper()
	e := srv.GetEcho()

	body := `{"name":"Dash","default_source_language":"en","target_languages":["fr"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	pid := created.ID

	ctx := t.Context()
	cs := srv.ContentStore
	seed := func(name, text string, translated bool) {
		b := &model.Block{ID: name + "-b1", Translatable: true}
		b.SetSourceText(text)
		if translated {
			b.SetTargetText(model.LocaleID("fr"), "traduit")
		}
		require.NoError(t, cs.StoreItem(ctx, pid, "main", &platstore.Item{ProjectID: pid, Name: name, Format: "json"}))
		require.NoError(t, cs.StoreBlocksForItem(ctx, pid, "main", name, []*model.Block{b}))
	}
	seed("c.json", "one", true)
	seed("a.json", "one two three four five", false)
	seed("b.json", "one two three", false)
	return pid
}

func getDashboard(t *testing.T, srv *Server, token, pid, query string) platstore.TranslationDashboardStats {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+pid+"/dashboard/main"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var stats platstore.TranslationDashboardStats
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats))
	return stats
}

func itemNames(stats platstore.TranslationDashboardStats) []string {
	names := make([]string, len(stats.ItemStats))
	for i, it := range stats.ItemStats {
		names[i] = it.ItemName
	}
	return names
}

// TestTranslationDashboardPagination asserts the dashboard endpoint pages and
// sorts item_stats server-side while locale totals, collection rollups, and
// item_total stay complete on every page — and that the legacy no-parameter
// request still returns the full list.
func TestTranslationDashboardPagination(t *testing.T) {
	srv, token := newTestServer(t)
	pid := seedDashboardProject(t, srv, token)

	// Legacy shape: no params → full item list plus the new total.
	full := getDashboard(t, srv, token, pid, "")
	assert.Len(t, full.ItemStats, 3)
	assert.Equal(t, 3, full.ItemTotal)
	assert.Equal(t, 9, full.TotalSourceWords)
	require.Len(t, full.LocaleStats, 1)

	// Project creation ensures a default collection, and StoreItem resolves an
	// unset collection_id to it — so every API-seeded item lands in the default
	// bucket, never the ungrouped one. (The ungrouped bucket exists only for
	// items whose named collection has no row; dashboard_coordinates_test
	// seeds that shape directly against the store.)
	require.Len(t, full.CollectionStats, 1)
	assert.False(t, full.CollectionStats[0].Ungrouped)
	assert.NotEmpty(t, full.CollectionStats[0].CollectionID)
	assert.Equal(t, "default", full.CollectionStats[0].CollectionName)
	assert.Equal(t, 3, full.CollectionStats[0].ItemCount)

	// First page sorted by words descending.
	page := getDashboard(t, srv, token, pid, "?limit=2&offset=0&sort=words&dir=desc")
	assert.Equal(t, []string{"a.json", "b.json"}, itemNames(page))
	assert.Equal(t, 3, page.ItemTotal, "item_total carries the full count on a page")
	assert.Len(t, page.LocaleStats, 1, "locale totals are complete on every page")
	assert.Equal(t, 9, page.TotalSourceWords, "global totals are complete on every page")

	// Second page.
	page = getDashboard(t, srv, token, pid, "?limit=2&offset=2&sort=words&dir=desc")
	assert.Equal(t, []string{"c.json"}, itemNames(page))

	// Completion sort: the fully translated item leads descending.
	page = getDashboard(t, srv, token, pid, "?limit=1&sort=completion&dir=desc")
	assert.Equal(t, []string{"c.json"}, itemNames(page))

	// Name sort ascending is the paging default.
	page = getDashboard(t, srv, token, pid, "?limit=2")
	assert.Equal(t, []string{"a.json", "b.json"}, itemNames(page))

	// Offset past the end yields an empty page, not an error.
	page = getDashboard(t, srv, token, pid, "?limit=2&offset=99")
	assert.Empty(t, page.ItemStats)
	assert.Equal(t, 3, page.ItemTotal)

	// Paging must not mutate the cached full result.
	full = getDashboard(t, srv, token, pid, "")
	assert.Len(t, full.ItemStats, 3)

	// Unknown sort column is rejected.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+pid+"/dashboard/main?sort=bogus", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
