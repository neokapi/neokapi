//go:build unix

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanContentTree_LinksOut plants links that lead out of the tree and
// asserts the scan lists none of what they reach, reads no ignore rules
// through them, and refuses a root .gitignore that is one.
func TestScanContentTree_LinksOut(t *testing.T) {
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.json"), []byte(`{"a":"b"}`), 0o644))
	// An ignore file outside that would hide app.json if it were read.
	require.NoError(t, os.WriteFile(filepath.Join(outside, "ignore"), []byte("app.json\n"), 0o644))

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "app.json"), []byte(`{"a":"b"}`), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.json"), filepath.Join(root, "src", "linked.json")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "out")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "ignore"), filepath.Join(root, "src", ".gitignore")))

	files, err := scanContentTree(root)
	require.NoError(t, err)
	assert.Equal(t, []string{"src/app.json"}, files, "nothing reached through a link out is listed, and the linked rules are not read")

	require.NoError(t, os.Symlink(filepath.Join(outside, "ignore"), filepath.Join(root, ".gitignore")))
	_, err = scanContentTree(root)
	assert.Error(t, err, "a root .gitignore that leads out of the tree is refused")
}
