package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookup(t *testing.T) {
	def, ok := Lookup("")
	require.True(t, ok)
	assert.Equal(t, "e5-small-int8", def.Name, "empty resolves to default")
	assert.True(t, def.Default)
	assert.Equal(t, 384, def.Dim)

	_, ok = Lookup("nope")
	assert.False(t, ok)
}

func TestCacheRootOverride(t *testing.T) {
	t.Setenv("KAPI_CHECK_CACHE", "/tmp/kapi-check-test")
	root, err := CacheRoot()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/kapi-check-test", root)
}

func TestResolveAndPresent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CHECK_CACHE", dir)

	p, spec, err := Resolve("")
	require.NoError(t, err)
	assert.Equal(t, "e5-small-int8", spec.Name)
	assert.Equal(t, filepath.Join(dir, "e5-small-int8", "model.onnx"), p.ONNX)
	assert.Equal(t, filepath.Join(dir, "e5-small-int8", "tokenizer.json"), p.Tokenizer)

	assert.False(t, Present(""), "nothing downloaded yet")
}

func TestDownloadComputesAndVerifiesSHA256(t *testing.T) {
	body := []byte("CHECK-MODEL-BYTES")
	sum := sha256.Sum256(body)
	good := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	t.Run("matching digest installs", func(t *testing.T) {
		dir := t.TempDir()
		dst := filepath.Join(dir, "model.onnx")
		d := &Downloader{HTTPClient: srv.Client()}
		require.NoError(t, d.download(srv.URL, dst, expected{sha256: good, size: int64(len(body))}))
		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, body, got)
	})

	t.Run("mismatching digest fails closed and leaves no file", func(t *testing.T) {
		dir := t.TempDir()
		dst := filepath.Join(dir, "model.onnx")
		d := &Downloader{HTTPClient: srv.Client()}
		err := d.download(srv.URL, dst, expected{sha256: "deadbeef"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integrity check failed")
		_, statErr := os.Stat(dst)
		assert.True(t, os.IsNotExist(statErr), "tampered file must not be committed")
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries, "no leftover temp files")
	})

	t.Run("unpinned installs with warning", func(t *testing.T) {
		dir := t.TempDir()
		dst := filepath.Join(dir, "model.onnx")
		var logs []string
		d := &Downloader{
			HTTPClient: srv.Client(),
			Logf:       func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) },
		}
		require.NoError(t, d.download(srv.URL, dst, expected{}))
		require.FileExists(t, dst)
		var warned bool
		for _, l := range logs {
			if strings.Contains(l, "UNVERIFIED") {
				warned = true
			}
		}
		assert.True(t, warned, "an unpinned download must log a clear unverified warning")
	})
}

func TestDownloadRejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	d := &Downloader{HTTPClient: srv.Client()}
	err := d.download(srv.URL, filepath.Join(t.TempDir(), "x"), expected{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status")
}
