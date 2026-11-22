package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project a person names, by -p or KAPI_PROJECT, resolves to its recipe
// when it holds one. When it does not, the error names the path as they
// wrote it and says which of the two named it.
func TestResolveRecipePathNamesAPathThatIsNotAProject(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: p\n"), 0o644))
	other := filepath.Join(root, "site.yaml")
	require.NoError(t, os.WriteFile(other, []byte("version: v1\nname: p\n"), 0o644))
	empty := t.TempDir()
	missing := filepath.Join(root, "nowhere")

	t.Run("the directory resolves to its recipe", func(t *testing.T) {
		got, err := ResolveRecipePath(root)
		require.NoError(t, err)
		assert.Equal(t, recipe, got)
	})
	t.Run("a recipe under another name resolves", func(t *testing.T) {
		got, err := ResolveRecipePath(other)
		require.NoError(t, err)
		assert.Equal(t, other, got)
	})
	for _, tc := range []struct {
		name, given string
		dir         bool
		want        string
	}{
		{"a missing path", missing, false, missing + " does not exist"},
		{"a missing recipe", filepath.Join(root, "gone.yaml"), false, filepath.Join(root, "gone.yaml") + " does not exist"},
		{"a directory with no recipe", empty, true, empty + " is not a kapi project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveRecipePath(tc.given)
			assert.Empty(t, got)
			var notProject *NotAProjectError
			require.ErrorAs(t, err, &notProject)
			assert.Equal(t, tc.given, notProject.Given)
			assert.Equal(t, tc.dir, notProject.Dir)
			assert.Contains(t, err.Error(), "-p "+tc.want)
		})
	}
	t.Run("KAPI_PROJECT names a missing path", func(t *testing.T) {
		t.Setenv(NoProjectEnvVar, "")
		t.Setenv(ProjectEnvVar, missing)
		_, err := ResolveRecipePath("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), ProjectEnvVar+" "+missing+" does not exist")
	})
	t.Run("KAPI_NO_PROJECT still resolves nothing", func(t *testing.T) {
		t.Setenv(NoProjectEnvVar, "1")
		t.Setenv(ProjectEnvVar, missing)
		got, err := ResolveRecipePath("")
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// A recipe named through a symbolic link resolves to the file the link names,
// and a file that is not a recipe is refused naming the path.
func TestResolveRecipePathFollowsALinkAndRefusesANonRecipe(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "project", RecipeFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(recipe), 0o755))
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: linked\n"), 0o644))
	link := filepath.Join(dir, "linked.yaml")
	require.NoError(t, os.Symlink(recipe, link))

	got, err := ResolveRecipePath(link)
	require.NoError(t, err)
	assert.Equal(t, recipe, got)

	notes := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(notes, []byte("- a list\n"), 0o644))
	_, err = ResolveRecipePath(notes)
	require.ErrorContains(t, err, notes+" is not a kapi recipe")
}
