package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// An uploaded file's preview marks each block by the key its row is stored
// under as source id, so the editor finds every stored block in the document
// it renders. A second upload of the same file keeps the rows and the markers.
func TestEditorUpload_PreviewMarksEachBlockByItsStoredKey(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	pid := createProject(t, srv, token)
	ctx := t.Context()
	const name = "guide.md"
	data := []byte("# Getting started\n\nInstall the tool.\n\nRun it on a file.\n")

	preview := func() []string {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+pid+"/preview/main?item="+name, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return previewMarkers(t, rec.Body.String())
	}
	storedKeys := func() []string {
		var keys []string
		for _, sb := range itemRows(t, srv, pid, name) {
			keys = append(keys, sb.SourceID)
		}
		sort.Strings(keys)
		return keys
	}

	_, err := editorAddFiles(ctx, srv.ContentStore, srv.FormatRegistry, pid, "main", map[string][]byte{name: data})
	require.NoError(t, err)
	first := storedKeys()
	require.Len(t, first, 3, "one row per block")
	assert.Equal(t, first, preview(), "each marker carries a stored row's source id")

	_, err = editorAddFiles(ctx, srv.ContentStore, srv.FormatRegistry, pid, "main", map[string][]byte{name: data})
	require.NoError(t, err)
	assert.Equal(t, first, storedKeys(), "a second upload keeps the rows")
	assert.Equal(t, first, preview(), "and the markers")
}
