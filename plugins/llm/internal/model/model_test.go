package model

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultModelName(t *testing.T) {
	assert.Equal(t, "gemma-4-e2b", DefaultModelName())
}

func TestLookup(t *testing.T) {
	s, ok := Lookup("")
	require.True(t, ok)
	assert.Equal(t, "gemma-4-e2b", s.Name, "empty name resolves to default")

	s, ok = Lookup("gemma-4-e2b")
	require.True(t, ok)
	assert.True(t, s.Default)
	assert.NotEmpty(t, s.Embed.RepoPath)
	assert.NotEmpty(t, s.Decoder.RepoPath)
	assert.NotEmpty(t, s.Vision.RepoPath)
	assert.NotEmpty(t, s.Audio.RepoPath)

	_, ok = Lookup("nope")
	assert.False(t, ok)
}

func TestFileBase(t *testing.T) {
	assert.Equal(t, "decoder_model_merged_q4.onnx", File{RepoPath: "onnx/decoder_model_merged_q4.onnx"}.Base())
	assert.Equal(t, "tokenizer.json", File{RepoPath: "tokenizer.json"}.Base())
}

func TestAllFilesCoversComponentsDataAndConfigs(t *testing.T) {
	s, _ := Lookup("gemma-4-e2b")
	files := s.allFiles()
	var bases []string
	for _, f := range files {
		bases = append(bases, f.Base())
	}
	// 4 components + 4 data siblings + tokenizer + 4 configs = 13.
	assert.Len(t, files, 13)
	assert.Contains(t, bases, "embed_tokens_q4.onnx")
	assert.Contains(t, bases, "decoder_model_merged_q4.onnx_data")
	assert.Contains(t, bases, "tokenizer.json")
	assert.Contains(t, bases, "generation_config.json")
}

func TestCacheRootPrecedence(t *testing.T) {
	t.Setenv("KAPI_LLM_CACHE", "/tmp/explicit")
	root, err := CacheRoot()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/explicit", root)

	t.Setenv("KAPI_LLM_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	root, err = CacheRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/xdg", "kapi", "models", "llm"), root)
}

// TestEnsureDownloadsAndCaches exercises the full download path against a fake
// HF server: every file is fetched once, stored under its basename, and a second
// Ensure is a pure cache hit (no further requests).
func TestEnsureDownloadsAndCaches(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		// Body is the request path so we can assert basename preservation.
		fmt.Fprintf(w, "content-of:%s", r.URL.Path)
	}))
	defer srv.Close()

	cache := t.TempDir()
	t.Setenv("KAPI_LLM_CACHE", cache)

	dl := &Downloader{HTTPClient: srv.Client()}
	// Point every file at the fake server by overriding the URL builder via a
	// custom spec download: we reuse Ensure but with a one-file model to keep
	// the test fast and deterministic.
	dl.HTTPClient = srv.Client()

	// Use a tiny spec routed at the test server.
	spec := Spec{
		Name:      "tiny",
		Repo:      "x/y",
		Default:   true,
		Embed:     File{RepoPath: "onnx/embed.onnx"},
		Decoder:   File{RepoPath: "onnx/decoder.onnx"},
		Data:      []File{{RepoPath: "onnx/decoder.onnx_data"}},
		Tokenizer: File{RepoPath: "tokenizer.json"},
	}
	orig := Registry
	Registry = []Spec{spec}
	defer func() { Registry = orig }()

	// Redirect hfURL by temporarily swapping the base host: easiest is to make
	// the downloader hit srv via a rewriting RoundTripper.
	dl.HTTPClient = &http.Client{Transport: rewriteHost{base: srv.URL, rt: srv.Client().Transport}}

	paths, err := dl.Ensure("tiny")
	require.NoError(t, err)

	// Files landed under their basenames.
	for _, p := range []string{paths.Embed, paths.Decoder, paths.Tokenizer} {
		b, rerr := os.ReadFile(p)
		require.NoError(t, rerr)
		assert.True(t, strings.HasPrefix(string(b), "content-of:"))
	}
	assert.Equal(t, "embed.onnx", filepath.Base(paths.Embed))
	dataPath := filepath.Join(paths.Dir, "decoder.onnx_data")
	assert.FileExists(t, dataPath)

	first := atomic.LoadInt64(&hits)
	assert.Equal(t, int64(4), first, "4 files fetched once each")

	// Second Ensure is a cache hit: no new requests.
	_, err = dl.Ensure("tiny")
	require.NoError(t, err)
	assert.Equal(t, first, atomic.LoadInt64(&hits), "cache hit makes no requests")
}

// rewriteHost sends every request to base, preserving the path, so the test can
// route the production HuggingFace URLs at a local server.
type rewriteHost struct {
	base string
	rt   http.RoundTripper
}

func (rw rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	u := req.URL
	// Keep the path (which carries the repo file path), swap scheme+host.
	newURL := rw.base + u.Path
	r2, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	rt := rw.rt
	if rt == nil {
		rt = http.DefaultTransport
	}
	return rt.RoundTrip(r2)
}
