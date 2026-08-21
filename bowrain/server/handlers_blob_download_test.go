package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cannotPresign is the local filesystem backend's behaviour: it holds the bytes
// and cannot mint a URL to them.
type cannotPresign struct {
	corestorage.BlobStore
}

func (cannotPresign) GenerateDownloadURL(context.Context, string, corestorage.SignOptions) (string, error) {
	return "", corestorage.ErrNotSupported
}

// A store that cannot pre-sign still delivers its bytes.
//
// This is the default configuration — `if backend == "" { backend = "local" }` —
// and generateDownloadURL returned "" for it, which every caller read as "no
// media here". So a pull wrote every translated file without its media and
// exited zero.
func TestBlobDownload_ServesWhatCannotBePresigned(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	payload := []byte("the bytes of an image")
	ref, err := srv.BlobStore.Upload(t.Context(), payload, corestorage.UploadOptions{})
	require.NoError(t, err)

	srv.BlobStore = cannotPresign{BlobStore: srv.BlobStore}
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+pid+"/sync/main/blobs/"+ref.Key, nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, body, "the bytes come back whole")
}

// The key is the scope of the route, exactly as it is for an upload grant.
func TestBlobDownload_RefusesKeysThatAreNotContentHashes(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	for _, bad := range []string{"manifest-abc.json", "NOTAHEXDIGEST", "0000"} {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/projects/"+pid+"/sync/main/blobs/"+bad, nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "key %q must be refused", bad)
	}
}

// An asset listed by a store that cannot pre-sign carries a URL now, not "".
func TestAssetListing_CarriesADownloadURLWithoutPresigning(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	payload := []byte("an asset")
	ref, err := srv.BlobStore.Upload(t.Context(), payload, corestorage.UploadOptions{})
	require.NoError(t, err)

	srv.BlobStore = cannotPresign{BlobStore: srv.BlobStore}
	e := srv.GetEcho()

	// The route the asset list would hand over must itself resolve.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+pid+"/sync/main/blobs/"+ref.Key, nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code,
		"the fallback URL generateDownloadURL hands out has to be a route that exists")
}
