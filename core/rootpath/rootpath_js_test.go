//go:build js

package rootpath_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/rootpath"
)

// Under js/wasm Contain is lexical, and the file system the tests run over
// (Node's) holds symbolic links. CheckRegular resolves a link itself: one that
// leads out of the root is refused, and one that stays inside is accepted.
func TestCheckRegularResolvesALinkWhereContainIsLexical(t *testing.T) {
	require.False(t, rootpath.Supported)
	base := t.TempDir()
	root := filepath.Join(base, "root")
	require.NoError(t, os.MkdirAll(root, 0o755))
	outside := filepath.Join(base, "outside.txt")
	inside := filepath.Join(root, "inside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))
	require.NoError(t, os.WriteFile(inside, []byte("inside"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "out.txt")),
		"the file system these tests run over holds links")
	require.NoError(t, os.Symlink("inside.txt", filepath.Join(root, "in.txt")))

	_, err := rootpath.CheckRegular(root, filepath.Join(root, "out.txt"))
	var escape *rootpath.EscapeError
	require.ErrorAs(t, err, &escape, "a link out of the root is refused")

	got, err := rootpath.CheckRegular(root, filepath.Join(root, "in.txt"))
	require.NoError(t, err)
	realInside, err := filepath.EvalSymlinks(inside)
	require.NoError(t, err)
	assert.Equal(t, realInside, got)
}
