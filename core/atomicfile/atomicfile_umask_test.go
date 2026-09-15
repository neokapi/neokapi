//go:build unix

package atomicfile_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/atomicfile"
)

// A new file is created under the umask, which is right for a file that did not
// exist and wrong for one that did: a file its group could write would come
// back without those bits. The replacement gives the file the mode it had,
// whatever the umask would take off.
func TestReplaceKeepsAModeTheUmaskWouldStrip(t *testing.T) {
	prev := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(prev) })

	path := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o666))
	// os.WriteFile creates under the umask too, so the mode is set here.
	require.NoError(t, os.Chmod(path, 0o666))

	_, err := atomicfile.ReplaceBytes(path, []byte("new"))
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o666), info.Mode().Perm())
}
