package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestModelCacheRootPrecedence(t *testing.T) {
	t.Setenv("KAPI_MODELS_CACHE", "/tmp/explicit")
	r, err := ModelCacheRoot()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/explicit", r)

	t.Setenv("KAPI_MODELS_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	r, err = ModelCacheRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/xdg", "kapi", "models"), r)

	dir, err := ModelDir("llm", "gemma", "1")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/xdg", "kapi", "models", "llm", "gemma", "1"), dir)
}

// servePath returns the body the test server emits for a request path, so the
// test can compute the matching pinned digest.
func servePath(p string) string { return "content-of:" + p }

func newModelServer(hits *int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		fmt.Fprint(w, servePath(r.URL.Path))
	}))
}

func TestEnsureModelDownloadsVerifiesCaches(t *testing.T) {
	var hits int64
	srv := newModelServer(&hits)
	defer srv.Close()

	cache := t.TempDir()
	t.Setenv("KAPI_MODELS_CACHE", cache)

	files := []manifest.ModelFile{
		{Path: "embed.onnx", URL: srv.URL + "/embed", SHA256: sha256hex(servePath("/embed"))},
		{Path: "decoder.onnx", URL: srv.URL + "/decoder", SHA256: sha256hex(servePath("/decoder"))},
		{Path: "tokenizer.json", URL: srv.URL + "/tok", SHA256: sha256hex(servePath("/tok"))},
	}
	asset := manifest.ModelAsset{ID: "gemma-4-e2b", Version: "1", Files: files}
	opts := ModelEnsureOptions{Plugin: "llm", HTTPClient: srv.Client()}

	dir, err := EnsureModel(context.Background(), asset, opts)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cache, "llm", "gemma-4-e2b", "1"), dir)

	// Files landed under their basenames with the served content.
	wants := map[string]string{"embed.onnx": "/embed", "decoder.onnx": "/decoder", "tokenizer.json": "/tok"}
	for name, p := range wants {
		b, e := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, e)
		assert.Equal(t, servePath(p), string(b))
	}
	assert.Equal(t, int64(3), atomic.LoadInt64(&hits), "each file fetched once")

	// Second call is a pure cache hit: no new requests, no temp/lock residue.
	_, err = EnsureModel(context.Background(), asset, opts)
	require.NoError(t, err)
	assert.Equal(t, int64(3), atomic.LoadInt64(&hits), "cache hit makes no requests")

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".download-", "no temp files left behind")
		assert.NotContains(t, e.Name(), ".lock", "no lock files left behind")
	}
}

func TestEnsureModelRejectsPathTraversal(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("KAPI_MODELS_CACHE", cache)
	for _, bad := range []string{"../escape", "a/b.onnx", "/abs.onnx", "..", "sub\\win.onnx"} {
		asset := manifest.ModelAsset{ID: "m", Version: "1", Files: []manifest.ModelFile{
			{Path: bad, URL: "http://unused", SHA256: sha256hex("x")},
		}}
		_, err := EnsureModel(context.Background(), asset, ModelEnsureOptions{Plugin: "llm"})
		require.Error(t, err, "path %q must be rejected", bad)
		assert.Contains(t, err.Error(), "invalid file path")
	}
}

func TestEnsureModelShaMismatchLeavesNoFile(t *testing.T) {
	var hits int64
	srv := newModelServer(&hits)
	defer srv.Close()
	cache := t.TempDir()
	t.Setenv("KAPI_MODELS_CACHE", cache)

	asset := manifest.ModelAsset{ID: "m", Version: "1", Files: []manifest.ModelFile{
		{Path: "a.onnx", URL: srv.URL + "/a", SHA256: sha256hex("not-what-the-server-sends")},
	}}
	_, err := EnsureModel(context.Background(), asset, ModelEnsureOptions{Plugin: "llm", HTTPClient: srv.Client()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integrity check failed")
	assert.NoFileExists(t, filepath.Join(cache, "llm", "m", "1", "a.onnx"))
}

func TestEnsureModelSizeMismatchLeavesNoFile(t *testing.T) {
	var hits int64
	srv := newModelServer(&hits)
	defer srv.Close()
	cache := t.TempDir()
	t.Setenv("KAPI_MODELS_CACHE", cache)

	// Pin a wrong (too-large) size so the size gate trips before the (correct) sha.
	asset := manifest.ModelAsset{ID: "m", Version: "1", Files: []manifest.ModelFile{
		{Path: "a.onnx", URL: srv.URL + "/a", SHA256: sha256hex(servePath("/a")), Size: 99999},
	}}
	_, err := EnsureModel(context.Background(), asset, ModelEnsureOptions{Plugin: "llm", HTTPClient: srv.Client()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size")
	assert.NoFileExists(t, filepath.Join(cache, "llm", "m", "1", "a.onnx"))
}
