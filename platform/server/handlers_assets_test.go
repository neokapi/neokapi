package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/storage/localblob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServerWithBlob(t *testing.T) (*Server, string) {
	t.Helper()
	srv, token := newTestServer(t)

	bs, err := localblob.New(t.TempDir())
	require.NoError(t, err)
	srv.BlobStore = bs

	return srv, token
}

func createTestProjectForAssets(t *testing.T, e http.Handler, token string) string {
	t.Helper()
	body := `{"name":"Asset Test","default_source_language":"en","target_languages":["fr"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp["id"].(string)
}

func TestAssetUploadURL(t *testing.T) {
	srv, token := newTestServerWithBlob(t)
	e := srv.GetEcho()
	auth := "Bearer " + token
	pid := createTestProjectForAssets(t, e, token)

	// Request upload URL — local backend returns empty URL (not supported).
	body := `{"blob_key":"aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa1111bbbb2222","content_type":"image/png"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/assets/upload-url", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadURLResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Exists)
	assert.Empty(t, resp.UploadURL) // local backend doesn't support pre-signed URLs
}

func TestAssetCRUDEndpoints(t *testing.T) {
	srv, token := newTestServerWithBlob(t)
	e := srv.GetEcho()
	auth := "Bearer " + token
	pid := createTestProjectForAssets(t, e, token)

	// Create asset.
	body := `{"blob_key":"deadbeef12345678","mime_type":"image/png","filename":"diagram.png","size_bytes":1024,"item_name":"doc.docx"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/assets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var asset AssetResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &asset))
	assert.NotEmpty(t, asset.ID)
	assert.Equal(t, "deadbeef12345678", asset.BlobKey)
	assert.Equal(t, "image/png", asset.MimeType)
	assert.Equal(t, "diagram.png", asset.Filename)
	assetID := asset.ID

	// List assets.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/assets?item_name=doc.docx", nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string][]AssetResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	assert.Len(t, listResp["assets"], 1)

	// Get single asset.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/assets/"+assetID, nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var gotAsset AssetResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &gotAsset))
	assert.Equal(t, assetID, gotAsset.ID)

	// Delete asset.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+pid+"/assets/"+assetID, nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify deleted.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/assets/"+assetID, nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAssetVariantEndpoints(t *testing.T) {
	srv, token := newTestServerWithBlob(t)
	e := srv.GetEcho()
	auth := "Bearer " + token
	pid := createTestProjectForAssets(t, e, token)

	// Create asset first.
	body := `{"blob_key":"varianttest1234","mime_type":"image/png","filename":"logo.png"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/assets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var asset AssetResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &asset))
	assetID := asset.ID

	// Create variant.
	body = `{"locale":"fr-FR","blob_key":"frvariant5678","mime_type":"image/png","status":"draft","size_bytes":512}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/assets/"+assetID+"/variants", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var variant AssetVariantResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &variant))
	assert.Equal(t, "fr-FR", variant.Locale)
	assert.Equal(t, "draft", variant.Status)

	// List variants.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/assets/"+assetID+"/variants", nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string][]AssetVariantResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	assert.Len(t, listResp["variants"], 1)
	assert.Equal(t, "fr-FR", listResp["variants"][0].Locale)
}

func TestAssetValidation(t *testing.T) {
	srv, token := newTestServerWithBlob(t)
	e := srv.GetEcho()
	auth := "Bearer " + token
	pid := createTestProjectForAssets(t, e, token)

	// Missing blob_key.
	body := `{"mime_type":"image/png"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/assets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Missing mime_type.
	body = `{"blob_key":"abc123"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/assets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Missing blob_key on upload-url.
	body = `{"content_type":"image/png"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/assets/upload-url", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Missing locale on variant.
	body = `{"blob_key":"abc"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/assets/fake-id/variants", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAssetNoBlobStore(t *testing.T) {
	srv, token := newTestServer(t)
	srv.BlobStore = nil // explicitly clear to test missing blob store
	e := srv.GetEcho()
	auth := "Bearer " + token
	pid := createTestProjectForAssets(t, e, token)

	// Upload URL should fail when no blob store.
	body := `{"blob_key":"test","content_type":"image/png"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/assets/upload-url", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
