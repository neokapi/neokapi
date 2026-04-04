package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderDocumentPreview_PreviewHTMLFallback(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)
	ctx := t.Context()

	// Preview routes are workspace-scoped: /api/v1/:ws/:pid/preview/main?item=...
	// The test workspace slug is "test" (created by newTestServer).
	previewURL := func(filename string) string {
		return "/api/v1/test/" + pid + "/preview/main?item=" + filename
	}

	t.Run("returns PreviewHTML directly when set", func(t *testing.T) {
		item := &store.Item{
			Name:        "rich.html",
			Format:      "html",
			ItemType:    "file",
			PreviewHTML: "<html><body><h1>Rich Preview</h1></body></html>",
			BlockIndex:  `{"blocks":[{"id":"b1","source_html":"<p>fallback</p>"}]}`,
		}
		require.NoError(t, srv.ContentStore.StoreItem(ctx, pid, "main", item))

		req := httptest.NewRequest(http.MethodGet, previewURL("rich.html"), nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "<html><body><h1>Rich Preview</h1></body></html>", rec.Body.String())
	})

	t.Run("falls back to BlockIndex preview when no PreviewHTML", func(t *testing.T) {
		item := &store.Item{
			Name:       "fallback.json",
			Format:     "json",
			ItemType:   "file",
			BlockIndex: `{"blocks":[{"id":"b1","source_html":"<p>generated</p>"}],"skeleton":["<p>","b1","</p>"]}`,
		}
		require.NoError(t, srv.ContentStore.StoreItem(ctx, pid, "main", item))

		req := httptest.NewRequest(http.MethodGet, previewURL("fallback.json"), nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		// The generated preview should be non-empty (built from the BlockIndex).
		assert.NotEmpty(t, rec.Body.String())
	})

	t.Run("returns OK with default BlockIndex when no PreviewHTML", func(t *testing.T) {
		// StoreItem defaults BlockIndex to "{}" when not provided.
		// BuildPreviewFromBlockIndex("{}") returns minimal boilerplate.
		item := &store.Item{
			Name:     "empty.txt",
			Format:   "plaintext",
			ItemType: "file",
		}
		require.NoError(t, srv.ContentStore.StoreItem(ctx, pid, "main", item))

		req := httptest.NewRequest(http.MethodGet, previewURL("empty.txt"), nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		// Should succeed; the response contains boilerplate from BuildPreviewFromBlockIndex.
		assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	})

	t.Run("returns 404 for nonexistent item", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, previewURL("nonexistent.json"), nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
