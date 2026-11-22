package host

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A path a person edits outside any project can be a symlink to the file's
// real home, which is how a checkout shares one file between trees (a
// dotfile linked into a dotfiles repository). The edit is the person's, so it
// is written through the link: the real file holds the edit and the link
// stays a link.
func TestApplyCommentThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture makes a symlink")
	}
	isolateCheckExecution(t)
	target := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
	require.NoError(t, os.Chmod(target, 0o640))
	home := t.TempDir()
	t.Chdir(home)
	link := filepath.Join(home, "parse.go")
	require.NoError(t, os.Symlink(target, link))

	out, _, _ := runApplyChangeSet(t, NewEnvCommand(t.Context(), "apply"), false, commentEntry(link, "func/Parse", nil, repairedParse))
	require.Len(t, out.Comments, 1)
	require.NotEmpty(t, out.Comments[0].Edits)
	assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status, out.Comments[0].Edits[0].Detail)

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is still a link")
	written, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Contains(t, string(written), "// Parse reads the input from an [io.Reader].",
		"the file the link points at holds the edit")
	perm, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), perm.Mode().Perm(), "the file keeps its mode")
}

// Inside a project the files are the project's content. A file that is a
// symbolic link leading out of the project is refused and the file it points
// at keeps its text; a link that stays inside is written through.
func TestApplyCommentInAProjectThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture makes a symlink")
	}
	isolateCheckExecution(t)
	newProject := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\nname: p\ndefaults:\n  source_language: en\n"), 0o644))
		t.Chdir(root)
		return root
	}

	t.Run("a link that leads out of the project is refused", func(t *testing.T) {
		outside := writeCheckInput(t, t.TempDir(), "parse.go", repairGo)
		root := newProject(t)
		link := filepath.Join(root, "parse.go")
		require.NoError(t, os.Symlink(outside, link))

		cmd := NewEnvCommand(t.Context(), "apply")
		cmd.Flags().String(projectFlagName, filepath.Join(root, project.RecipeFileName), "")
		out, _, _ := runApplyChangeSet(t, cmd, false, commentEntry(link, "func/Parse", nil, repairedParse))
		refusal := refusalOf(t, out)
		assert.Contains(t, refusal.Message, "leads outside the root")
		written, err := os.ReadFile(outside)
		require.NoError(t, err)
		assert.Equal(t, repairGo, string(written), "the file the link points at keeps its text")
	})

	t.Run("a link that stays inside the project is written through", func(t *testing.T) {
		root := newProject(t)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "real"), 0o755))
		target := writeCheckInput(t, filepath.Join(root, "real"), "parse.go", repairGo)
		link := filepath.Join(root, "parse.go")
		require.NoError(t, os.Symlink(filepath.Join("real", "parse.go"), link))

		cmd := NewEnvCommand(t.Context(), "apply")
		cmd.Flags().String(projectFlagName, filepath.Join(root, project.RecipeFileName), "")
		out, _, _ := runApplyChangeSet(t, cmd, false, commentEntry(link, "func/Parse", nil, repairedParse))
		require.Len(t, out.Comments, 1)
		require.NotEmpty(t, out.Comments[0].Edits)
		assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status, out.Comments[0].Edits[0].Detail)
		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is still a link")
		written, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Contains(t, string(written), "// Parse reads the input from an [io.Reader].")
	})
}
