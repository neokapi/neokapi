//go:build unix

package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreproj "github.com/neokapi/neokapi/core/project"
)

// SaveRecipe writes the recipe through the folder that holds it: a recipe
// that is a link to a file outside the folder is refused, and the file it
// points at keeps what it held.
func TestSaveRecipeRefusesALinkOutOfTheFolder(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	secret := filepath.Join(outside, "kapi.yaml")
	require.NoError(t, os.WriteFile(secret, []byte("held: outside\n"), 0o644))
	recipe := filepath.Join(root, coreproj.RecipeFileName)
	require.NoError(t, os.Symlink(secret, recipe))

	err := SaveRecipe(recipe, &Recipe{Name: "t"})
	require.ErrorContains(t, err, "leads outside the root")
	got, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, "held: outside\n", string(got))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no staged file is left beside the recipe")
}
