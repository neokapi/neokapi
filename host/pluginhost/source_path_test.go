//go:build unix

package pluginhost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// A plugin daemon opens the path it is given without a check of its own, so
// the host sends only the resolved path of the file it opened the document
// from, and never the document's URI.
func TestDaemonSourcePath(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "doc.xml")
	require.NoError(t, os.WriteFile(real, []byte("<a/>"), 0o644))
	link := filepath.Join(dir, "link.xml")
	require.NoError(t, os.Symlink("doc.xml", link))

	t.Run("a URI alone is never sent", func(t *testing.T) {
		assert.Empty(t, daemonSourcePath(&model.RawDocument{URI: real}))
	})

	t.Run("the file the host opened, with its links resolved", func(t *testing.T) {
		f, err := os.Open(link)
		require.NoError(t, err)
		defer f.Close()
		want, err := filepath.EvalSymlinks(real)
		require.NoError(t, err)
		assert.Equal(t, want, daemonSourcePath(&model.RawDocument{URI: "member.xml", ReaderAt: f}))
	})

	t.Run("a path that no longer names the held file is not sent", func(t *testing.T) {
		f, err := os.Open(real)
		require.NoError(t, err)
		defer f.Close()
		other := filepath.Join(dir, "other.xml")
		require.NoError(t, os.WriteFile(other, []byte("<b/>"), 0o644))
		require.NoError(t, os.Rename(other, real))
		assert.Empty(t, daemonSourcePath(&model.RawDocument{ReaderAt: f}))
	})
}
