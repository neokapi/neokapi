package model

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupAndDefault(t *testing.T) {
	assert.Equal(t, "sat-3l-sm", DefaultModelName())

	s, ok := Lookup("")
	require.True(t, ok)
	assert.Equal(t, "sat-3l-sm", s.Name, "empty resolves to default")

	s, ok = Lookup("sat-12l-sm")
	require.True(t, ok)
	assert.Equal(t, "segment-any-text/sat-12l-sm", s.Repo)
	assert.Equal(t, "facebookAI/xlm-roberta-base", s.TokenizerRepo)

	_, ok = Lookup("sat-bogus")
	assert.False(t, ok)
}

func TestCacheRootPrecedence(t *testing.T) {
	t.Setenv("KAPI_SAT_CACHE", "/tmp/explicit")
	root, err := CacheRoot()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/explicit", root)

	t.Setenv("KAPI_SAT_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	root, err = CacheRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/xdg", "kapi", "models", "sat"), root)
}

func TestHFURL(t *testing.T) {
	assert.Equal(t,
		"https://huggingface.co/segment-any-text/sat-3l-sm/resolve/main/model.onnx",
		hfURL("segment-any-text/sat-3l-sm", "model.onnx"))
}

func TestRegistryUsesFloat32Model(t *testing.T) {
	for _, s := range Registry {
		assert.Equal(t, "model.onnx", s.ONNXFile, "%s must use the float32 export", s.Name)
		assert.Equal(t, "facebookAI/xlm-roberta-base", s.TokenizerRepo)
	}
}

func TestDownloadAtomicAndChecksum(t *testing.T) {
	body := "ONNX-MODEL-BYTES"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "model.onnx")
	d := &Downloader{HTTPClient: srv.Client()}
	require.NoError(t, d.download(srv.URL, dst))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))

	// No leftover temp files.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestDownloadRejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	d := &Downloader{HTTPClient: srv.Client()}
	err := d.download(srv.URL, filepath.Join(t.TempDir(), "x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status")
}

func TestEnsureFileSkipsWhenPresent(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "model.onnx")
	require.NoError(t, os.WriteFile(dst, []byte("already-here"), 0o644))

	// Server that would fail the test if hit.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be hit when file already present")
	}))
	defer srv.Close()

	d := &Downloader{HTTPClient: srv.Client()}
	require.NoError(t, d.ensureFile(dst, srv.URL))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "already-here", string(got))
}

func TestEnsureFileConcurrentSingleDownload(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "model.onnx")

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := &Downloader{HTTPClient: srv.Client()}
			assert.NoError(t, d.ensureFile(dst, srv.URL))
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, hits, "lock must collapse concurrent downloads to one fetch")

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(got))
}

func TestAcquireLockReclaimsStale(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "model.onnx.lock")
	require.NoError(t, os.WriteFile(lock, nil, 0o644))
	// Backdate well past the (short) timeout.
	old := mustOld(t)
	require.NoError(t, os.Chtimes(lock, old, old))

	d := &Downloader{LockTimeout: 50 * time.Millisecond}
	unlock, err := d.acquireLock(lock)
	require.NoError(t, err, "stale lock must be reclaimed")
	unlock()
	_, statErr := os.Stat(lock)
	assert.True(t, os.IsNotExist(statErr), "unlock removes the lock file")
}

func mustOld(t *testing.T) time.Time {
	t.Helper()
	return time.Now().Add(-1 * time.Hour)
}
