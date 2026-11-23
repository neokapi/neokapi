//go:build unix

package rootpath

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A file reached through a link is refused even when the link stays inside
// the root, at the file or at a directory on the way; a plain file opens.
func TestOpenRegularNoLink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "d"), 0o700))
	plain := filepath.Join(root, "d", "f")
	require.NoError(t, os.WriteFile(plain, []byte("x"), 0o600))
	require.NoError(t, os.Symlink(plain, filepath.Join(root, "link")))
	require.NoError(t, os.Symlink(filepath.Join(root, "d"), filepath.Join(root, "dirlink")))

	f, err := OpenRegularNoLink(root, plain)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = OpenRegularNoLink(root, filepath.Join(root, "link"))
	require.ErrorContains(t, err, "symbolic link")
	_, err = OpenRegularNoLink(root, filepath.Join(root, "dirlink", "f"))
	require.ErrorContains(t, err, "symbolic link")
}
