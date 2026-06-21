package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultModelName(t *testing.T) {
	assert.Equal(t, "e5-small-int8", DefaultModelName())
}

func TestLookup(t *testing.T) {
	s, ok := Lookup("")
	require.True(t, ok)
	assert.Equal(t, "e5-small-int8", s.Name)
	assert.True(t, s.Default)
	assert.Equal(t, "1", s.Version)
	assert.Equal(t, 384, s.Dim)

	_, ok = Lookup("nope")
	assert.False(t, ok)
}

func TestModelsRootPrecedence(t *testing.T) {
	t.Setenv("KAPI_CHECK_MODELS_ROOT", "/tmp/explicit")
	r, err := ModelsRoot()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/explicit", r)

	t.Setenv("KAPI_CHECK_MODELS_ROOT", "")
	t.Setenv("KAPI_MODELS_CACHE", "/tmp/cache")
	r, err = ModelsRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/cache", "check"), r)

	t.Setenv("KAPI_MODELS_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	r, err = ModelsRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/xdg", "kapi", "models", "check"), r)
}

func stage(t *testing.T, root, id, version string) string {
	t.Helper()
	dir := filepath.Join(root, id, version)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, f := range []string{onnxFile, tokenizerFile} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644))
	}
	return dir
}

func TestResolveInDirAndPresent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KAPI_CHECK_MODELS_ROOT", root)

	assert.False(t, Present("e5-small-int8"), "absent before staging")

	dir := stage(t, root, "e5-small-int8", "1")
	p, err := ResolveInDir("e5-small-int8", root)
	require.NoError(t, err)
	assert.Equal(t, dir, p.Dir)
	assert.Equal(t, filepath.Join(dir, "model.onnx"), p.ONNX)
	assert.Equal(t, filepath.Join(dir, "tokenizer.json"), p.Tokenizer)
	assert.Equal(t, 384, p.Dim)
	assert.True(t, Present("e5-small-int8"))
}

func TestResolveInDirMissingFile(t *testing.T) {
	root := t.TempDir()
	dir := stage(t, root, "e5-small-int8", "1")
	require.NoError(t, os.Remove(filepath.Join(dir, "model.onnx")))

	_, err := ResolveInDir("e5-small-int8", root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model.onnx")
	assert.Contains(t, err.Error(), "kapi models pull check")
}

func TestResolveInDirUnknownModel(t *testing.T) {
	_, err := ResolveInDir("nope", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown model")
}
