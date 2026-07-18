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

// seedSummaryProject creates a project via the API and seeds two items with
// blocks through the ContentStore: a.json with two translatable blocks (2 + 4
// words) and b.json with one translatable block (3 words) plus one
// non-translatable block. Totals: 2 items, 4 blocks, 9 translatable words.
func seedSummaryProject(t *testing.T, srv *Server, token string) string {
	t.Helper()
	e := srv.GetEcho()

	body := `{"name":"Summary","default_source_language":"en","target_languages":["fr","de"]}`
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

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello world")
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceText("Three more source words")
	b2.SetTargetText(model.LocaleID("fr"), "Trois mots source de plus")
	require.NoError(t, cs.StoreItem(ctx, pid, "main", &platstore.Item{ProjectID: pid, Name: "a.json", Format: "json"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, pid, "main", "a.json", []*model.Block{b1, b2}))

	b3 := &model.Block{ID: "b3", Translatable: true}
	b3.SetSourceText("One two three")
	b4 := &model.Block{ID: "b4", Translatable: false}
	b4.SetSourceText("not counted words here")
	require.NoError(t, cs.StoreItem(ctx, pid, "main", &platstore.Item{ProjectID: pid, Name: "b.json", Format: "json"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, pid, "main", "b.json", []*model.Block{b3, b4}))

	return pid
}

// TestListWorkspaceProjectsSummary asserts the default projects-list response
// is the lightweight summary: precomputed aggregates, no embedded per-item
// rows, and ?view=full restores the legacy embedded items[].
func TestListWorkspaceProjectsSummary(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	pid := seedSummaryProject(t, srv, token)

	// Default view: summary aggregates, empty items.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/projects", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var summary []ProjectInfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &summary))
	require.Len(t, summary, 1)
	p := summary[0]
	assert.Equal(t, pid, p.ID)
	assert.Empty(t, p.Items, "summary view must not embed per-item rows")
	assert.Empty(t, p.Collections, "summary view must not embed collections")
	assert.Empty(t, p.Streams, "summary view must not embed streams")
	assert.Equal(t, 2, p.ItemCount)
	assert.Equal(t, 4, p.BlockCount, "block count includes non-translatable blocks")
	assert.Equal(t, 2+4+3, p.WordCount, "word count sums translatable source words only")
	assert.Equal(t, 1, p.StreamCount, "project creation ensures the main stream")

	// The raw payload must not carry the heavy arrays either (items is the
	// always-present key; it must be empty, and collections/streams absent).
	var raw []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	assert.JSONEq(t, "[]", string(raw[0]["items"]))
	assert.NotContains(t, raw[0], "collections")
	assert.NotContains(t, raw[0], "streams")

	// ?view=full keeps the legacy embedded item rows with per-item counts.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/test/projects?view=full", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var full []ProjectInfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &full))
	require.Len(t, full, 1)
	require.Len(t, full[0].Items, 2)
	byName := map[string]ProjectItemResponse{}
	for _, it := range full[0].Items {
		byName[it.Name] = it
	}
	assert.Equal(t, 2, byName["a.json"].BlockCount)
	assert.Equal(t, 6, byName["a.json"].WordCount)
	assert.Equal(t, 2, byName["b.json"].BlockCount)
	assert.Equal(t, 3, byName["b.json"].WordCount)
	// Full view carries the same aggregates the summary computes.
	assert.Equal(t, 2, full[0].ItemCount)
	assert.Equal(t, 4, full[0].BlockCount)
	assert.Equal(t, 9, full[0].WordCount)
}

// TestGetProjectCarriesAggregates asserts the single-project response keeps
// the full arrays and now also carries the same aggregate fields, so detail
// consumers and summary consumers read one coherent shape.
func TestGetProjectCarriesAggregates(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	pid := seedSummaryProject(t, srv, token)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+pid, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var info ProjectInfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
	assert.Len(t, info.Items, 2, "detail view keeps the embedded items")
	assert.Equal(t, 2, info.ItemCount)
	assert.Equal(t, 4, info.BlockCount)
	assert.Equal(t, 9, info.WordCount)
	assert.Equal(t, 1, info.StreamCount)
}
