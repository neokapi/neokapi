package host

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A path a person edits can be a symlink to the file's real home, which is how
// a checkout shares one file between trees. Writing a temporary file and
// renaming it onto the link replaces the link with a regular file: the real
// file keeps the old text, and the edit reports what it wrote.
func TestApplyCommentThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture makes a symlink")
	}
	isolateCheckExecution(t)
	target := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
	require.NoError(t, os.Chmod(target, 0o640))
	link := filepath.Join(t.TempDir(), "parse.go")
	require.NoError(t, os.Symlink(target, link))

	out, _, _ := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false, commentEntry(link, "func/Parse", nil, repairedParse))
	require.Len(t, out.Comments, 1)
	require.NotEmpty(t, out.Comments[0].Edits)
	assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status, out.Comments[0].Edits[0].Detail)

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the edit replaced the symlink with a regular file")

	written, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Contains(t, string(written), "// Parse reads the input from an [io.Reader].",
		"the file the link points at holds the edit")

	perm, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), perm.Mode().Perm(), "the file keeps its mode")
}
