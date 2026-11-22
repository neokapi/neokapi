package host

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A comment edit applied under a root (the project, or the working directory
// outside one) reads and writes only files that resolve inside that root. A
// file that is a symbolic link leading out of the root is refused and the
// file it points at keeps its text. A link that stays inside the root is
// written through, and stays a link.
func TestApplyCommentThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture makes a symlink")
	}
	isolateCheckExecution(t)

	t.Run("a link that leads out of the root is refused", func(t *testing.T) {
		outside := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		root := t.TempDir()
		t.Chdir(root)
		link := filepath.Join(root, "parse.go")
		require.NoError(t, os.Symlink(outside, link))

		out, _, _ := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false, commentEntry(link, "func/Parse", nil, repairedParse))
		refusal := refusalOf(t, out)
		assert.Contains(t, refusal.Message, "leads outside the root")

		written, err := os.ReadFile(outside)
		require.NoError(t, err)
		assert.Equal(t, repairGo, string(written), "the file the link points at keeps its text")
	})

	t.Run("a link that stays inside the root is written through", func(t *testing.T) {
		root := t.TempDir()
		t.Chdir(root)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "real"), 0o755))
		target := writeCheckInput(t, filepath.Join(root, "real"), "parse.go", repairGo)
		require.NoError(t, os.Chmod(target, 0o640))
		link := filepath.Join(root, "parse.go")
		require.NoError(t, os.Symlink(filepath.Join("real", "parse.go"), link))

		out, _, _ := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false, commentEntry(link, "func/Parse", nil, repairedParse))
		require.Len(t, out.Comments, 1)
		require.NotEmpty(t, out.Comments[0].Edits)
		assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status, out.Comments[0].Edits[0].Detail)

		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is still a link")
		written, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Contains(t, string(written), "// Parse reads the input from an [io.Reader].")
		perm, err := os.Stat(target)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), perm.Mode().Perm(), "the file keeps its mode")
	})
}
