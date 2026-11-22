//go:build unix

package safeio_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/safeio"
)

// A FIFO beside the document is refused at once rather than waited on, and a
// link that leads out of the directory is refused.
func TestOpenInRootRefusesWhatIsNotARegularFile(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "doc")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "rules.xml"), 0o600))
	_, err := safeio.OpenInRoot(dir, "rules.xml")
	require.ErrorContains(t, err, "not a regular file")

	secret := filepath.Join(base, "secret.xml")
	require.NoError(t, os.WriteFile(secret, []byte("<x/>"), 0o644))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, "linked.xml")))
	_, err = safeio.OpenInRoot(dir, "linked.xml")
	require.Error(t, err)
}
