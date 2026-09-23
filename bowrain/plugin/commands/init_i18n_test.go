package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyFrameworkPreset_NeokapiI18nCleanLayout proves `kapi init --preset
// neokapi-i18n` scaffolds the clean nested layout: source KBF catalogs under
// i18n/src/ and per-locale targets under i18n/{lang}/. It then round-trips
// through InitProject so the cache directory and its ignore rule are asserted
// too.
func TestApplyFrameworkPreset_NeokapiI18nCleanLayout(t *testing.T) {
	recipe := &project.Recipe{}
	require.NoError(t, applyFrameworkPreset(recipe, "neokapi-i18n"))

	require.Len(t, recipe.Collections, 1)
	assert.Equal(t, "i18n/src/**/*.kbf.json", recipe.Collections[0].Path)
	assert.Equal(t, "i18n/{lang}/{path}.kbf.json", recipe.Collections[0].Target)
	require.NotNil(t, recipe.Collections[0].Format)
	assert.Equal(t, "kbf", recipe.Collections[0].Format.Name)
	assert.Nil(t, recipe.Defaults.Voice, "a voice is bound by name once the store holds one")

	dir := t.TempDir()
	proj, err := project.InitProject(dir, recipe)
	require.NoError(t, err)

	assert.DirExists(t, proj.StateDir(), "init creates the cache directory")
	assert.NoFileExists(t, filepath.Join(proj.StateDir(), "manifest.yaml"))
	assert.Empty(t, proj.FlowsDirPath(), "init names no flows directory")

	gi, err := os.ReadFile(filepath.Join(proj.StateDir(), ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(gi), "the whole cache directory stays out of version control")
}

// The plugin's init proposes collections from the tree the way `kapi init`
// does, and leaves a recipe that already names some alone.
func TestProposeCollections_FillsAnEmptyRecipe(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Demo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "a.md"), []byte("# A\n"), 0o644))

	recipe := &project.Recipe{}
	proposed, err := proposeCollections(dir, recipe)
	require.NoError(t, err)
	require.Len(t, proposed, 2)
	var paths []string
	for _, c := range recipe.Collections {
		paths = append(paths, c.Path)
	}
	assert.Equal(t, []string{"README.md", "docs/*.md"}, paths)

	again, err := proposeCollections(dir, recipe)
	require.NoError(t, err)
	assert.Empty(t, again)
	assert.Len(t, recipe.Collections, 2)
}
