package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover epic 006 task 5 (server side): the upload handlers no
// longer silently drop files that cannot be imported. The response gains
// "skipped": [{"name","reason"}] naming each file that was not imported,
// while importable files in the same batch still land.

func newUploadTestServer(t *testing.T) (*Server, *bstore.PostgresStore) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return &Server{ContentStore: cs, FormatRegistry: reg, wsStores: newWorkspaceStores()}, cs
}

func seedUploadProject(t *testing.T, cs *bstore.PostgresStore) *platstore.Project {
	t.Helper()
	proj := &platstore.Project{
		Name:                  "upload-proj",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		WorkspaceID:           "ws-1",
		Properties:            map[string]string{},
	}
	require.NoError(t, cs.CreateProject(t.Context(), proj))
	return proj
}

// multipartFiles builds a multipart body with the given name → content files
// under the "files" field the upload handlers read.
func multipartFiles(t *testing.T, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.CreateFormFile("files", name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

type uploadResponse struct {
	Items []struct {
		Name string `json:"name"`
	} `json:"items"`
	Skipped []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"skipped"`
}

// TestHandleUploadFilesReportsSkipped: uploading a bogus-extension file
// alongside a good JSON file imports the JSON and lists the bogus file in
// "skipped" with a reason.
func TestHandleUploadFilesReportsSkipped(t *testing.T) {
	srv, cs := newUploadTestServer(t)
	proj := seedUploadProject(t, cs)

	body, contentType := multipartFiles(t, map[string]string{
		"app.json":  `{"greeting": "Hello"}`,
		"notes.xyz": "not importable",
	})

	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/"+proj.ID+"/files", body)
	r.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("acme", proj.ID)
	c.Set("project_permissions", platauth.PermAll)

	require.NoError(t, srv.HandleUploadFiles(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp uploadResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	itemNames := make([]string, 0, len(resp.Items))
	for _, it := range resp.Items {
		itemNames = append(itemNames, it.Name)
	}
	assert.Contains(t, itemNames, "app.json", "the importable file must still land")
	assert.NotContains(t, itemNames, "notes.xyz")

	require.Len(t, resp.Skipped, 1)
	assert.Equal(t, "notes.xyz", resp.Skipped[0].Name)
	assert.NotEmpty(t, resp.Skipped[0].Reason, "the skip reason must be surfaced")
	assert.Contains(t, resp.Skipped[0].Reason, ".xyz")
}

// TestHandleUploadFilesNoSkipped: a clean upload carries no skipped entries
// (the field is omitted, which the UI treats as none).
func TestHandleUploadFilesNoSkipped(t *testing.T) {
	srv, cs := newUploadTestServer(t)
	proj := seedUploadProject(t, cs)

	body, contentType := multipartFiles(t, map[string]string{
		"app.json": `{"greeting": "Hello"}`,
	})

	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/"+proj.ID+"/files", body)
	r.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("acme", proj.ID)
	c.Set("project_permissions", platauth.PermAll)

	require.NoError(t, srv.HandleUploadFiles(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, present := raw["skipped"]
	assert.False(t, present, "skipped must be omitted when every file imported")
}

// TestHandleUploadToCollectionReportsSkipped: the collections upload handler
// carries the same per-file skip reporting.
func TestHandleUploadToCollectionReportsSkipped(t *testing.T) {
	srv, cs := newUploadTestServer(t)
	proj := seedUploadProject(t, cs)

	coll := &platstore.Collection{
		ProjectID: proj.ID,
		Name:      "docs",
		Kind:      platstore.CollectionUploaded,
		ItemLabel: "item",
	}
	require.NoError(t, cs.CreateCollection(t.Context(), coll))

	body, contentType := multipartFiles(t, map[string]string{
		"page.html": "<p>Hello</p>",
		"notes.xyz": "not importable",
	})

	e := echo.New()
	r := httptest.NewRequest(http.MethodPost,
		"/api/v1/acme/"+proj.ID+"/collections/"+coll.ID+"/files", body)
	r.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "cid")
	c.SetParamValues("acme", proj.ID, coll.ID)
	c.Set("project_permissions", platauth.PermAll)

	require.NoError(t, srv.HandleUploadToCollection(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp uploadResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	itemNames := make([]string, 0, len(resp.Items))
	for _, it := range resp.Items {
		itemNames = append(itemNames, it.Name)
	}
	assert.Contains(t, itemNames, "page.html")

	require.Len(t, resp.Skipped, 1)
	assert.Equal(t, "notes.xyz", resp.Skipped[0].Name)
	assert.NotEmpty(t, resp.Skipped[0].Reason)
}
