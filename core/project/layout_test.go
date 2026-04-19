package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLayout_walksUpFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "my-app.kapi"), []byte("id: my-app\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "deep"), 0o755))

	layout, err := project.ResolveLayout(filepath.Join(root, "src", "deep"))
	require.NoError(t, err)
	assert.Equal(t, root, layout.Root)
	assert.Equal(t, filepath.Join(root, "my-app.kapi"), layout.RecipePath)
	assert.Equal(t, filepath.Join(root, ".kapi"), layout.StateDir)
}

func TestResolveLayout_noProjectFound(t *testing.T) {
	root := t.TempDir()
	_, err := project.ResolveLayout(root)
	assert.ErrorIs(t, err, project.ErrNoProject)
}

func TestResolveLayout_ambiguousMultipleRecipes(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "one.kapi"), []byte("id: one\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "two.kapi"), []byte("id: two\n"), 0o644))

	_, err := project.ResolveLayout(root)
	assert.ErrorIs(t, err, project.ErrAmbiguousLayout)
}

func TestResolveLayout_stateWithoutRecipe(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	_, err := project.ResolveLayout(root)
	assert.ErrorIs(t, err, project.ErrRecipeMissing)
}

func TestResolveLayout_startIsAFile(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "my-app.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte("id: my-app\n"), 0o644))

	layout, err := project.ResolveLayout(recipe)
	require.NoError(t, err)
	assert.Equal(t, recipe, layout.RecipePath)
}

func TestLayoutFor_explicitRecipePath(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "my-app.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte("id: my-app\n"), 0o644))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	assert.Equal(t, recipe, layout.RecipePath)
	assert.Equal(t, filepath.Join(root, ".kapi"), layout.StateDir)
}

func TestLayoutFor_rejectsNonKapiExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wrong.yaml")
	require.NoError(t, os.WriteFile(path, []byte("id: x\n"), 0o644))
	_, err := project.LayoutFor(path)
	assert.Error(t, err)
}

func TestEnsureLayout_createsStateDir(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "my-app.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte("id: my-app\n"), 0o644))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	info, err := os.Stat(layout.StateDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// State manifest round-trip.
func TestStateManifest_roundTrip(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "my-app.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte("id: my-app\n"), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)

	orig := &project.StateManifest{
		Generator: project.StateGenerator{ID: "kapi", Version: "0.5.0"},
		Project:   project.StateProjectRef{ID: "my-app", Path: "../my-app.kapi"},
		Blocks: map[string]project.StateBlockStats{
			"ui": {Count: 42, SHA256: "abc123"},
		},
	}
	require.NoError(t, project.SaveState(layout, orig))

	got, err := project.LoadState(layout)
	require.NoError(t, err)
	assert.Equal(t, "my-app", got.Project.ID)
	assert.Equal(t, 42, got.Blocks["ui"].Count)
	assert.Equal(t, "abc123", got.Blocks["ui"].SHA256)
	assert.NotEmpty(t, got.UpdatedAt)
}

func TestLoadState_missingFileReturnsNil(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "my-app.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte("id: my-app\n"), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	got, err := project.LoadState(layout)
	require.NoError(t, err)
	assert.Nil(t, got)
}
