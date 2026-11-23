//go:build unix

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A merge writes the package's layout beneath the merge directory, through
// it as the root: a directory link there that leads out is refused, and no
// file or directory is made through it.
func TestMergeFromKpzWritesNothingThroughALinkOutOfTheMergeDirectory(t *testing.T) {
	a, work := kpzWorkspace(t, "fr")
	mergeDir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(mergeDir, "fr")))
	t.Chdir(mergeDir)

	err := a.MergeFromKpz(t.Context(), work, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "leads outside the root")
	entries, rerr := os.ReadDir(outside)
	require.NoError(t, rerr)
	assert.Empty(t, entries, "nothing is written through the link")
}

// A layout the merge directory holds is written there, its directories made
// at commit.
func TestMergeFromKpzWritesBeneathTheMergeDirectory(t *testing.T) {
	a, work := kpzWorkspace(t, "fr")
	mergeDir := t.TempDir()
	t.Chdir(mergeDir)
	require.NoError(t, a.MergeFromKpz(t.Context(), work, ""))
	assert.FileExists(t, filepath.Join(mergeDir, "fr", "messages.json"))
}
