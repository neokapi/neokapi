//go:build unix

package host

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordOne records a one-block document for path and reports whether the
// cache accepted the recording.
func recordOne(t *testing.T, c *docCache, path string) bool {
	t.Helper()
	rec := c.RecordDocument(path, "k", "json")
	if rec == nil {
		return false
	}
	require.NoError(t, rec.Add(&model.Part{Type: model.PartBlock, Resource: mkBlock("a", "Bonjour")}))
	require.NoError(t, rec.Commit())
	return true
}

// TestDocCache_RefusesSourceOutsideProject plants a target file that is a link
// out of the project and asserts the project's cache neither records nor
// serves it. A file swapped for such a link after it was recorded is a miss
// too, and so is a FIFO.
func TestDocCache_RefusesSourceOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"a":"Bonjour"}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))

	c, err := OpenDocCache(project.LayoutAt(root).CacheDir())
	require.NoError(t, err)
	defer c.Close()
	assert.Equal(t, root, c.root, "a project's cache directory binds the cache to the project")

	linked := filepath.Join(root, "locales", "fr.json")
	require.NoError(t, os.Symlink(outside, linked))
	assert.False(t, recordOne(t, c, linked), "a link out of the project is not recorded")
	assert.Nil(t, c.OpenDocument(linked, "k"), "a link out of the project is not served")

	// Record the file while it is a regular file, then swap in the link.
	require.NoError(t, os.Remove(linked))
	require.NoError(t, os.WriteFile(linked, []byte(`{"a":"Bonjour"}`), 0o644))
	require.True(t, recordOne(t, c, linked), "a regular file inside the project is recorded")
	require.NotNil(t, c.OpenDocument(linked, "k"))
	require.NoError(t, os.Remove(linked))
	require.NoError(t, os.Symlink(outside, linked))
	assert.Nil(t, c.OpenDocument(linked, "k"), "a recorded file replaced by a link out is a miss")

	fifo := filepath.Join(root, "locales", "de.json")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))
	assert.False(t, recordOne(t, c, fifo), "a FIFO is not recorded")
	assert.Nil(t, c.OpenDocument(fifo, "k"))
}

// TestDocCache_RefusesStateLinkOutside asserts a project whose cache
// directory is a link out of the project opens no cache.
func TestDocCache_RefusesStateLinkOutside(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	layout := project.LayoutAt(root)
	require.NoError(t, os.MkdirAll(layout.WorkDir(), 0o755))
	require.NoError(t, os.Symlink(outside, layout.CacheDir()))

	_, err := OpenDocCache(layout.CacheDir())
	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(outside, "docs"))
	assert.True(t, os.IsNotExist(statErr), "nothing is created outside the project")
}
