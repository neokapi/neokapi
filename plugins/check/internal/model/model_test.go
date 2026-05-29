package model

import (
	"path/filepath"
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
