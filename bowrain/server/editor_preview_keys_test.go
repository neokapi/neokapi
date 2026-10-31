package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// katBlockID reads the id of each <kat-block> marker in a preview.
var katBlockID = regexp.MustCompile(`<kat-block id="([^"]*)"`)

func previewMarkers(t *testing.T, html string) []string {
	t.Helper()
	var ids []string
	for _, m := range katBlockID.FindAllStringSubmatch(html, -1) {
		ids = append(ids, m[1])
	}
	sort.Strings(ids)
	return ids
}

// servedPreviewMarkers reads the markers of the preview the server serves for
// an item.
func servedPreviewMarkers(t *testing.T, srv *Server, token, pid, name string) []string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+pid+"/preview/main?item="+name, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return previewMarkers(t, rec.Body.String())
}

// storedSourceIDs is the sorted source id of each stored row of an item.
func storedSourceIDs(t *testing.T, srv *Server, pid, name string) []string {
	t.Helper()
	var keys []string
	for _, sb := range itemRows(t, srv, pid, name) {
		keys = append(keys, sb.SourceID)
	}
	sort.Strings(keys)
	return keys
}

// An uploaded file's preview marks each block by the key its row is stored
// under as source id, so the editor finds every stored block in the document
// it renders. One upload publishes one item.created. A second upload of the
// same file keeps the rows, the translation on one of them and the markers.
func TestEditorUpload_PreviewMarksEachBlockByItsStoredKey(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ctx := t.Context()
	const name = "guide.md"
	data := []byte("# Getting started\n\nInstall the tool.\n\nRun it on a file.\n")

	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)
	srv.ContentStore = event.NewEventEmittingStore(srv.ContentStore, bus)
	// upload returns how many item.created events one upload published.
	upload := func() int {
		created := 0
		sub := bus.SubscribeAll(func(ev platev.Event) {
			if ev.Type == platev.EventItemCreated && ev.Data["item_name"] == name {
				created++
			}
		})
		_, err := editorAddFiles(ctx, srv.ContentStore, srv.FormatRegistry, pid, "main", map[string][]byte{name: data})
		require.NoError(t, err)
		// Unsubscribe returns once the handler has seen every event delivered.
		bus.Unsubscribe(sub)
		return created
	}

	assert.Equal(t, 1, upload(), "one upload publishes one item.created")
	first := storedSourceIDs(t, srv, pid, name)
	require.Len(t, first, 3, "one row per block")
	item, err := srv.ContentStore.GetItem(ctx, pid, "main", name)
	require.NoError(t, err)
	assert.Equal(t, first, previewMarkers(t, item.PreviewHTML), "the stored preview marks each row's source id")
	assert.Equal(t, first, servedPreviewMarkers(t, srv, token, pid, name), "and so does the served one")

	rows := itemRows(t, srv, pid, name)
	var translated string
	for id := range rows {
		if translated == "" || id < translated {
			translated = id
		}
	}
	_, err = srv.ContentStore.UpdateBlock(ctx, pid, "main", translated, func(sb *venue.StoredBlock) error {
		sb.Block.SetTargetText(model.LocaleID("fr"), "Bonjour")
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, 1, upload(), "a second upload publishes one item.created")
	after := itemRows(t, srv, pid, name)
	assert.Len(t, after, len(rows), "a second upload adds no row and removes none")
	for id := range rows {
		assert.Contains(t, after, id)
	}
	if kept := after[translated]; assert.NotNil(t, kept) {
		assert.Equal(t, "Bonjour", kept.Block.TargetText(model.LocaleID("fr")), "the translation stays")
	}
	assert.Equal(t, first, servedPreviewMarkers(t, srv, token, pid, name), "the markers stay")
}

// A fetched item has no stored preview and no block index, so the server
// serves a listing of its stored blocks. The listing marks each block by its
// stored source id.
func TestFetchedItem_PreviewMarksEachBlockByItsStoredKey(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	const name = "locales/en/app.json"

	require.NoError(t, connector.StoreFetched(t.Context(), srv.ContentStore, pid, "main", []*connector.ContentItem{{
		ID: name, Path: name, Format: "json",
		Blocks: []*model.Block{
			model.NewBlock("greeting", "Hello from the app"),
			model.NewBlock("farewell", "Goodbye from the app"),
		},
	}}))

	keys := storedSourceIDs(t, srv, pid, name)
	require.Len(t, keys, 2)
	item, err := srv.ContentStore.GetItem(t.Context(), pid, "main", name)
	require.NoError(t, err)
	require.Empty(t, item.PreviewHTML, "a fetch stores no preview")
	assert.Equal(t, keys, servedPreviewMarkers(t, srv, token, pid, name))
}
