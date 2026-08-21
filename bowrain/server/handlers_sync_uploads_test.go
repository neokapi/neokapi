package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// presigningStore is a blob store that can grant presigned PUTs, wrapping one
// that cannot. It stands in for S3 in a test that must not reach S3.
type presigningStore struct {
	corestorage.BlobStore
	granted []string
}

func (p *presigningStore) GenerateUploadURL(_ context.Context, key string, _ corestorage.SignOptions) (string, error) {
	p.granted = append(p.granted, key)
	return "https://objects.example/" + key + "?signature=stub", nil
}

func requestUploads(t *testing.T, e http.Handler, projectID, authHeader string, hashes []string) (map[string]any, *httptest.ResponseRecorder) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"hashes": hashes})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+projectID+"/sync/main/push/uploads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	out := map[string]any{}
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return out, rec
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// A store that can presign grants a write per hash it does not already hold.
func TestSyncPushUploads_GrantsDirectWrites(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// One chunk already in storage, one not.
	present := []byte("already here")
	ref, err := srv.BlobStore.Upload(t.Context(), present, corestorage.UploadOptions{})
	require.NoError(t, err)
	presentHash := ref.Key

	store := &presigningStore{BlobStore: srv.BlobStore}
	srv.BlobStore = store
	e := srv.GetEcho()

	missingHash := hashOf([]byte("not here yet"))
	out, rec := requestUploads(t, e, pid, authHeader, []string{presentHash, missingHash})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, "direct", out["transport"])

	urls, _ := out["urls"].(map[string]any)
	have, _ := out["have"].([]any)

	assert.Len(t, urls, 1)
	assert.Contains(t, urls, missingHash)
	assert.NotContains(t, urls, presentHash,
		"an object the venue already holds is the same bytes, by construction — re-uploading it would move the same content twice")
	require.Len(t, have, 1)
	assert.Equal(t, presentHash, have[0])
}

// A store with no presigning — the self-hosted local backend — says so, and the
// producer proxies. That is what the transport distinction exists to say.
func TestSyncPushUploads_FallsBackToTheProxy(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	out, rec := requestUploads(t, e, pid, authHeader, []string{hashOf([]byte("anything"))})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "proxy", out["transport"])
	assert.Empty(t, out["urls"])
}

// A presigned URL is a write to one key, so the shape of the key IS the scope
// of the grant. A caller that could name an arbitrary key would be handed a
// write to it.
func TestSyncPushUploads_RefusesKeysThatAreNotContentHashes(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	authHeader := "Bearer " + token

	store := &presigningStore{BlobStore: srv.BlobStore}
	srv.BlobStore = store
	e := srv.GetEcho()

	for _, bad := range []string{
		"../../etc/passwd",
		"manifest-abc.json",
		"NOTAHEXDIGESTNOTAHEXDIGESTNOTAHEXDIGESTNOTAHEXDIGESTNOTAHEXDIGE",
		"",
	} {
		_, rec := requestUploads(t, e, pid, authHeader, []string{bad})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "key %q must be refused", bad)
	}
	assert.Empty(t, store.granted, "no grant is minted for a key that is not a content hash")
}

// A push's bytes are verified by hash at commit, so an object written under a
// name that is not its own content refuses the push rather than landing.
func TestSyncPushCommit_RefusesAChunkThatIsNotInStorage(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// A hash the producer claims it uploaded, and did not.
	commitBody, err := json.Marshal(map[string]any{
		"upload_id": "u1",
		"chunks": []map[string]any{{
			"index": 0, "content_type": "blocks",
			"hash": hashOf([]byte("never uploaded")), "record_count": 1, "byte_size": 10,
		}},
		"items": json.RawMessage(`[{"name":"en.json","format":"json"}]`),
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+pid+"/sync/main/push/commit", bytes.NewReader(commitBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body, _ := io.ReadAll(rec.Body)
	assert.Contains(t, string(body), "not found in storage")
}

// The grant and the commit meet at the object store: a client PUTs to the URL
// it was given, and the commit finds the object under the hash it names.
//
// The two halves are tested apart everywhere else — the grant here, the PUT in
// the client's own tests — and apart is exactly where a key-derivation
// disagreement hides. This drives the real handlers against a store that
// presigns to a real endpoint, so the key the URL grants and the key the commit
// looks for have to be the same key.
func TestSyncPushUploads_TheGrantedWriteIsWhatTheCommitFinds(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	backing := srv.BlobStore
	// An endpoint that accepts the granted PUT and stores the bytes where the
	// venue looks for them.
	objects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := backing.Upload(r.Context(), body, corestorage.UploadOptions{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer objects.Close()

	srv.BlobStore = &endpointPresigningStore{BlobStore: backing, endpoint: objects.URL}
	e := srv.GetEcho()

	payload := []byte("a chunk of content")
	hash := hashOf(payload)

	out, rec := requestUploads(t, e, pid, authHeader, []string{hash})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "direct", out["transport"])
	urls, _ := out["urls"].(map[string]any)
	url, ok := urls[hash].(string)
	require.True(t, ok, "a hash the venue does not hold gets a grant")

	// The client's half: PUT the bytes to the URL, carrying no credential.
	putReq, err := http.NewRequestWithContext(t.Context(), http.MethodPut, url, bytes.NewReader(payload))
	require.NoError(t, err)
	putResp, err := http.DefaultClient.Do(putReq)
	require.NoError(t, err)
	require.NoError(t, putResp.Body.Close())
	require.Equal(t, http.StatusOK, putResp.StatusCode)

	// The venue's half: the commit finds the object under the hash it names.
	exists, err := srv.BlobStore.Exists(t.Context(), hash)
	require.NoError(t, err)
	assert.True(t, exists, "the key the grant wrote to is the key the commit looks for")

	// And asking again now reports it held, so a retry re-uploads nothing.
	out, rec = requestUploads(t, e, pid, authHeader, []string{hash})
	require.Equal(t, http.StatusOK, rec.Code)
	have, _ := out["have"].([]any)
	require.Len(t, have, 1)
	assert.Equal(t, hash, have[0])
}

// endpointPresigningStore presigns to a URL a test can actually PUT to.
type endpointPresigningStore struct {
	corestorage.BlobStore
	endpoint string
}

func (p *endpointPresigningStore) GenerateUploadURL(_ context.Context, key string, _ corestorage.SignOptions) (string, error) {
	return p.endpoint + "/" + key + "?signature=stub", nil
}
