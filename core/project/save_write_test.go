package project

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stagedBeside lists what a write left beside the recipe besides the recipe
// and the names the test put there.
func stagedBeside(t *testing.T, dir string, keep ...string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var left []string
	for _, e := range entries {
		if e.Name() == RecipeFileName || slices.Contains(keep, e.Name()) {
			continue
		}
		left = append(left, e.Name())
	}
	return left
}

// A link beside the recipe, at the name a temporary file of a fixed name
// would take, is never opened: the save stages under a fresh name, created
// exclusively, and the file the link leads to keeps its bytes.
func TestSave_LeavesALinkBesideTheRecipeAlone(t *testing.T) {
	path := saveLockRecipe(t)
	victim := filepath.Join(t.TempDir(), "victim")
	require.NoError(t, os.WriteFile(victim, []byte("untouched\n"), 0o644))
	require.NoError(t, os.Symlink(victim, path+".tmp"))

	proj, err := Load(path)
	require.NoError(t, err)
	proj.Name = "renamed"
	require.NoError(t, Save(path, proj))

	got, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "untouched\n", string(got), "the file the link leads to keeps its bytes")
	info, err := os.Lstat(path + ".tmp")
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link stays a link")
	info, err = os.Lstat(path)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	assert.Empty(t, stagedBeside(t, filepath.Dir(path), filepath.Base(path)+".tmp"), "no temporary file is left")
}

// A recipe that is a link out of its directory is refused, on the read as on
// the write, and the file the link leads to keeps its bytes. One that leads
// to a file inside the directory is written through, as every file kapi
// writes beneath a root is: the file it points at is replaced and the link
// stays a link.
func TestSave_ARecipeThatIsALink(t *testing.T) {
	t.Run("out of its directory: refused", func(t *testing.T) {
		outside := saveLockRecipe(t)
		before, err := os.ReadFile(outside)
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), RecipeFileName)
		require.NoError(t, os.Symlink(outside, path))

		err = SaveOver(t.Context(), path, &KapiProject{Name: "renamed"})
		require.Error(t, err)
		got, err := os.ReadFile(outside)
		require.NoError(t, err)
		assert.Equal(t, before, got, "nothing is written outside the directory")
	})
	t.Run("inside its directory: written through", func(t *testing.T) {
		dir := t.TempDir()
		real := filepath.Join(dir, "real.yaml")
		require.NoError(t, Save(real, &KapiProject{Name: "app", Defaults: Defaults{SourceLanguage: "en"}}))
		path := filepath.Join(dir, RecipeFileName)
		require.NoError(t, os.Symlink("real.yaml", path))

		proj, err := Load(path)
		require.NoError(t, err)
		proj.Name = "renamed"
		require.NoError(t, Save(path, proj))
		got, err := os.ReadFile(real)
		require.NoError(t, err)
		assert.True(t, strings.Contains(string(got), "renamed"))
		info, err := os.Lstat(path)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link stays a link")
	})
}

// The saved recipe keeps the mode the file it replaces had.
func TestSave_KeepsTheModeOfTheRecipe(t *testing.T) {
	path := saveLockRecipe(t)
	require.NoError(t, os.Chmod(path, 0o600))
	proj, err := Load(path)
	require.NoError(t, err)
	proj.Name = "renamed"
	require.NoError(t, Save(path, proj))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
