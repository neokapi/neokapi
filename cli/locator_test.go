package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEntryLocator(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "bundle.zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("PK\x03\x04 dummy"), 0o644))
	plain := filepath.Join(dir, "weird!name.json")
	require.NoError(t, os.WriteFile(plain, []byte("{}"), 0o644))

	t.Run("archive!entry parses", func(t *testing.T) {
		loc, ok := ParseEntryLocator(zipPath + "!locales/en.json")
		require.True(t, ok)
		assert.Equal(t, zipPath, loc.Archive)
		assert.Equal(t, "locales/en.json", loc.Entry)
	})
	t.Run("leading slash on entry trimmed", func(t *testing.T) {
		loc, ok := ParseEntryLocator(zipPath + "!/a.json")
		require.True(t, ok)
		assert.Equal(t, "a.json", loc.Entry)
	})
	t.Run("plain path with bang but no container ext is not a locator", func(t *testing.T) {
		_, ok := ParseEntryLocator(plain)
		assert.False(t, ok)
	})
	t.Run("nonexistent archive is not a locator", func(t *testing.T) {
		_, ok := ParseEntryLocator(filepath.Join(dir, "missing.zip") + "!a.json")
		assert.False(t, ok)
	})
	t.Run("empty entry is not a locator", func(t *testing.T) {
		_, ok := ParseEntryLocator(zipPath + "!")
		assert.False(t, ok)
	})
}

func TestEntryLabel(t *testing.T) {
	withEntry := model.NewBlock("b1", "x")
	withEntry.Properties[model.PropContainerEntry] = "locales/en.json"
	assert.Equal(t, "bundle.zip!locales/en.json", entryLabel("bundle.zip", withEntry))

	plain := model.NewBlock("b2", "x")
	assert.Equal(t, "doc.json", entryLabel("doc.json", plain))
	assert.Equal(t, "doc.json", entryLabel("doc.json", nil))
}

func TestAnyContainerInput(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "a.zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("PK\x03\x04"), 0o644))
	assert.True(t, anyContainerInput([]string{"x.md", "a.tar"}))
	assert.True(t, anyContainerInput([]string{zipPath + "!x.json"}))
	assert.False(t, anyContainerInput([]string{"x.md", "y.json"}))
}
