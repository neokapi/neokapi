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

// brokenRecipes are recipes with a YAML error, each with what its parse error
// names: the line, and the problem.
var brokenRecipes = []struct {
	name, body string
	says       []string
}{
	{"duplicate key", "version: v1\nname: dup\ndefaults:\n  target_languages: [fr]\n  target_languages: [de]\n",
		[]string{"line 5", "already defined"}},
	{"tab indentation", "version: v1\nname: tab\ndefaults:\n\ttarget_languages: [fr]\n",
		[]string{"line 4", "tab"}},
	{"type error", "version: v1\nname: typed\ndefaults:\n  target_languages: fr\n",
		[]string{"line 4", "cannot unmarshal"}},
}

// A recipe with a YAML error is still the recipe: the resolution returns it,
// and the load reports the parser's error with its line, naming the file.
func TestResolveRecipePathKeepsARecipeWithAYAMLError(t *testing.T) {
	for _, tc := range brokenRecipes {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			recipe := filepath.Join(dir, RecipeFileName)
			require.NoError(t, os.WriteFile(recipe, []byte(tc.body), 0o644))

			got, err := ResolveRecipePath(recipe)
			require.NoError(t, err, "a recipe with a YAML error is not refused as not a recipe")
			_, err = Load(got)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "not a kapi recipe")
			assert.Contains(t, err.Error(), recipe)
			for _, want := range tc.says {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}
