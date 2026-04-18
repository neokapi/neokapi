package project_test

import (
	"bytes"
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

// Snapshot / Open round-trip.
func TestSnapshotOpen_roundTrip(t *testing.T) {
	// Build a project with recipe + state + a sidecar file.
	src := t.TempDir()
	recipePath := filepath.Join(src, "my-app.kapi")
	require.NoError(t, os.WriteFile(recipePath, []byte(
		"version: v1\n"+
			"id: my-app\n"+
			"sourceLocale: en\n"+
			"targetLocales: [fr, de]\n"+
			"content:\n"+
			"  - collection: ui\n",
	), 0o644))

	srcLayout, err := project.LayoutFor(recipePath)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(srcLayout))

	// Drop a sidecar into collections/ui/targets/fr.json.
	sidecarDir := filepath.Join(srcLayout.StateDir, "collections", "ui", "targets")
	require.NoError(t, os.MkdirAll(sidecarDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sidecarDir, "fr.json"), []byte(`{"h1":"bonjour"}`), 0o644))

	// Snapshot.
	var buf bytes.Buffer
	require.NoError(t, project.Snapshot(srcLayout, &buf, project.SnapshotOptions{}))
	require.NotZero(t, buf.Len())

	// Open into a fresh directory.
	target := t.TempDir()
	reader := bytes.NewReader(buf.Bytes())
	layout, err := project.Open(reader, int64(buf.Len()), target)
	require.NoError(t, err)

	// Recipe landed with the project id as filename stem.
	assert.Equal(t, filepath.Join(target, "my-app.kapi"), layout.RecipePath)
	recipeData, err := os.ReadFile(layout.RecipePath)
	require.NoError(t, err)
	assert.Contains(t, string(recipeData), "id: my-app")
	assert.Contains(t, string(recipeData), "sourceLocale: en")

	// State manifest landed.
	state, err := project.LoadState(layout)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "my-app", state.Project.ID)

	// Sidecar survived round-trip.
	sidecar, err := os.ReadFile(filepath.Join(layout.StateDir, "collections", "ui", "targets", "fr.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"h1":"bonjour"}`, string(sidecar))
}

func TestOpen_rejectsNonEmptyTarget(t *testing.T) {
	target := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(target, "existing.txt"), []byte("x"), 0o644))

	// Build a minimal zip in memory.
	src := t.TempDir()
	recipe := filepath.Join(src, "my-app.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte("id: my-app\n"), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))
	var buf bytes.Buffer
	require.NoError(t, project.Snapshot(layout, &buf, project.SnapshotOptions{}))

	_, err = project.Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()), target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not empty")
}

func TestArchiveManifest_kindValidation(t *testing.T) {
	bad := []byte("schemaVersion: 1\nkind: wrong\n")
	_, err := project.DecodeArchiveManifest(bad)
	assert.Error(t, err)
}
