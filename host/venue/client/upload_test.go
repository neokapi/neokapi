package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// objectStore stands in for S3: it accepts PUTs at any path and records the
// bytes it received under the key the path names.
type objectStore struct {
	*httptest.Server
	mu      sync.Mutex
	objects map[string][]byte
	auth    []string
}

func newObjectStore(t *testing.T) *objectStore {
	t.Helper()
	o := &objectStore{objects: map[string][]byte{}}
	o.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		o.mu.Lock()
		o.objects[strings.TrimPrefix(r.URL.Path, "/")] = body
		o.auth = append(o.auth, r.Header.Get("Authorization"))
		o.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(o.Close)
	return o
}

func (o *objectStore) stored() map[string][]byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make(map[string][]byte, len(o.objects))
	maps.Copy(out, o.objects)
	return out
}

// A push whose venue grants presigned PUTs writes its chunks to object storage
// and never sends a byte of content through the API.
func TestPushWritesChunksToObjectStorage(t *testing.T) {
	objects := newObjectStore(t)

	var proxied int
	var gotCommit PushCommitRequest
	var grantedHashes []string

	venue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				UploadID: "up1", Status: "diff_computed", Transport: TransportDirect,
			})
		case strings.HasSuffix(r.URL.Path, "/push/uploads"):
			var req struct {
				Hashes []string `json:"hashes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			urls := map[string]string{}
			for _, h := range req.Hashes {
				grantedHashes = append(grantedHashes, h)
				urls[h] = objects.URL + "/" + h
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"transport": TransportDirect, "urls": urls, "have": []string{},
			})
		case strings.Contains(r.URL.Path, "/push/chunks/"):
			proxied++
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/push/commit"):
			_ = json.NewDecoder(r.Body).Decode(&gotCommit)
			_ = json.NewEncoder(w).Encode(map[string]string{"push_id": "p1", "status": "queued"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer venue.Close()

	c := NewClaimTokenClient(venue.URL, "proj1", "tok")
	_, err := c.Push(context.Background(), map[string][]*model.Block{
		"a.json": {
			{ID: "b1", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "Hello"}}}},
			{ID: "b2", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "World"}}}},
		},
	}, nil, nil, nil)
	require.NoError(t, err)

	assert.Zero(t, proxied, "no content byte goes through the API when the venue grants direct writes")

	stored := objects.stored()
	require.Len(t, stored, 1, "one chunk, written straight to storage")
	require.Len(t, gotCommit.Chunks, 1)

	// The key IS the content's hash: that is what makes the grant safe, and
	// what lets the commit verify by name.
	for key, body := range stored {
		sum := sha256.Sum256(body)
		assert.Equal(t, hex.EncodeToString(sum[:]), key,
			"an object is written under the hash of its own bytes")
		assert.Equal(t, key, gotCommit.Chunks[0].Hash, "the manifest names what was written")
		assert.Equal(t, int64(len(body)), gotCommit.Chunks[0].ByteSize)
	}
	assert.Equal(t, []string{gotCommit.Chunks[0].Hash}, grantedHashes)

	// The presigned URL is the whole grant. Sending our own credential
	// alongside it tells the object store something it has no business seeing,
	// and some backends refuse the request outright.
	for _, header := range objects.auth {
		assert.Empty(t, header, "a direct write carries no credential of ours")
	}
}

// A venue whose blob store is a directory has no presigning, and the push
// proxies exactly as it did before there was another option.
func TestPushProxiesWhenTheVenueCannotPresign(t *testing.T) {
	var proxiedBodies [][]byte
	var gotCommit PushCommitRequest

	venue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				UploadID: "up1", Status: "diff_computed", Transport: TransportProxy,
			})
		case strings.HasSuffix(r.URL.Path, "/push/uploads"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"transport": TransportProxy, "urls": map[string]string{}, "have": []string{},
			})
		case strings.Contains(r.URL.Path, "/push/chunks/"):
			body, _ := io.ReadAll(r.Body)
			proxiedBodies = append(proxiedBodies, body)
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/push/commit"):
			_ = json.NewDecoder(r.Body).Decode(&gotCommit)
			_ = json.NewEncoder(w).Encode(map[string]string{"push_id": "p1", "status": "queued"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer venue.Close()

	c := NewClaimTokenClient(venue.URL, "proj1", "tok")
	_, err := c.Push(context.Background(), map[string][]*model.Block{
		"a.json": {{ID: "b1", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "Hello"}}}}},
	}, nil, nil, nil)
	require.NoError(t, err)

	require.Len(t, proxiedBodies, 1)
	require.Len(t, gotCommit.Chunks, 1)
	sum := sha256.Sum256(proxiedBodies[0])
	assert.Equal(t, hex.EncodeToString(sum[:]), gotCommit.Chunks[0].Hash,
		"the proxied path names its chunk the same way, so the venue verifies it the same way")
}

// A chunk the venue already holds is not sent again. The key is the content, so
// holding it is holding exactly those bytes — a push retried after a failed
// commit re-uploads nothing.
func TestPushSkipsChunksTheVenueAlreadyHolds(t *testing.T) {
	objects := newObjectStore(t)
	var proxied int

	venue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				UploadID: "up1", Status: "diff_computed", Transport: TransportDirect,
			})
		case strings.HasSuffix(r.URL.Path, "/push/uploads"):
			var req struct {
				Hashes []string `json:"hashes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"transport": TransportDirect,
				"urls":      map[string]string{},
				"have":      req.Hashes, // the venue holds every one already
			})
		case strings.Contains(r.URL.Path, "/push/chunks/"):
			proxied++
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/push/commit"):
			_ = json.NewEncoder(w).Encode(map[string]string{"push_id": "p1", "status": "queued"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer venue.Close()

	c := NewClaimTokenClient(venue.URL, "proj1", "tok")
	_, err := c.Push(context.Background(), map[string][]*model.Block{
		"a.json": {{ID: "b1", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "Hello"}}}}},
	}, nil, nil, nil)
	require.NoError(t, err)

	assert.Empty(t, objects.stored(), "content the venue already holds is not written again")
	assert.Zero(t, proxied)
}
