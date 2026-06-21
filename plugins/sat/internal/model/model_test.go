package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultModelName(t *testing.T) {
	assert.Equal(t, "sat-3l-sm", DefaultModelName())
}

func TestLookupAndDefault(t *testing.T) {
	s, ok := Lookup("")
	require.True(t, ok)
	assert.Equal(t, "sat-3l-sm", s.Name, "empty name resolves to default")
	assert.True(t, s.Default)
	assert.Equal(t, "1", s.Version)

	s, ok = Lookup("sat-12l-sm")
	require.True(t, ok)
	assert.False(t, s.Default)
	assert.Equal(t, "1", s.Version)

	_, ok = Lookup("nope")
	assert.False(t, ok)
}

// stage writes non-empty placeholders for a model's files under root.
func stage(t *testing.T, root, id, version string) string {
	t.Helper()
	dir := filepath.Join(root, id, version)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, f := range []string{onnxFile, tokenizerFile} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644))
	}
	return dir
}

func TestResolveInDirMapsStagedFiles(t *testing.T) {
	root := t.TempDir()
	dir := stage(t, root, "sat-3l-sm", "1")

	p, err := ResolveInDir("sat-3l-sm", root)
	require.NoError(t, err)
	assert.Equal(t, dir, p.Dir)
	assert.Equal(t, filepath.Join(dir, "model.onnx"), p.ONNX)
	assert.Equal(t, filepath.Join(dir, "tokenizer.json"), p.Tokenizer)

	// The non-default model resolves under its own id/version dir.
	dir12 := stage(t, root, "sat-12l-sm", "1")
	p, err = ResolveInDir("sat-12l-sm", root)
	require.NoError(t, err)
	assert.Equal(t, dir12, p.Dir)
}

func TestResolveInDirEmptyRoot(t *testing.T) {
	_, err := ResolveInDir("sat-3l-sm", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no model root")
}

func TestResolveInDirUnknownModel(t *testing.T) {
	_, err := ResolveInDir("nope", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown model")
}

func TestResolveInDirMissingFile(t *testing.T) {
	root := t.TempDir()
	dir := stage(t, root, "sat-3l-sm", "1")
	require.NoError(t, os.Remove(filepath.Join(dir, "model.onnx")))

	_, err := ResolveInDir("sat-3l-sm", root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model.onnx")
	assert.Contains(t, err.Error(), "kapi models pull sat/sat-3l-sm")
}
