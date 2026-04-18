package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAppForTest builds a minimally-wired App for exercising the
// init/snapshot/open commands. These commands don't touch the tool
// or plugin registries, so bare InitRegistries is sufficient.
func newAppForTest(t *testing.T) *App {
	t.Helper()
	app := &App{}
	app.InitRegistries()
	return app
}

func TestInitCmd_scaffoldsProject(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := app.NewInitCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dir", dir, "--name", "my-app", "--source-locale", "en", "--target-locale", "fr"})
	require.NoError(t, cmd.Execute())

	// Recipe + state dir both exist.
	recipe := filepath.Join(dir, "my-app.kapi")
	info, err := os.Stat(recipe)
	require.NoError(t, err)
	assert.False(t, info.IsDir())

	stateDir := filepath.Join(dir, ".kapi")
	info, err = os.Stat(stateDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// State manifest was written with the project id.
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	state, err := project.LoadState(layout)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "my-app", state.Project.ID)

	// Recipe contains the target locale.
	recipeData, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(recipeData), "sourceLocale: en")
	assert.Contains(t, string(recipeData), "fr")
}

func TestInitCmd_refusesExistingRecipe(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	// Pre-create the recipe file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.kapi"), []byte("version: v1\n"), 0o644))

	cmd := app.NewInitCmd()
	cmd.SetArgs([]string{"--dir", dir, "--name", "existing"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to overwrite")
}

func TestSnapshotOpenCmd_roundTrip(t *testing.T) {
	app := newAppForTest(t)

	// 1. Scaffold a project.
	src := t.TempDir()
	init := app.NewInitCmd()
	var buf bytes.Buffer
	init.SetOut(&buf)
	init.SetErr(&buf)
	init.SetArgs([]string{"--dir", src, "--name", "round", "--source-locale", "en"})
	require.NoError(t, init.Execute())

	// Add a sidecar to exercise the copy path.
	sidecarDir := filepath.Join(src, ".kapi", "collections", "ui", "targets")
	require.NoError(t, os.MkdirAll(sidecarDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sidecarDir, "fr.json"), []byte(`{"h1":"bonjour"}`), 0o644))

	// 2. Snapshot to .klz.
	archivePath := filepath.Join(t.TempDir(), "round.klz")
	snap := app.NewSnapshotCmd()
	snap.SetOut(&buf)
	snap.SetErr(&buf)
	snap.SetArgs([]string{"--project", filepath.Join(src, "round.kapi"), "--out", archivePath})
	require.NoError(t, snap.Execute())

	info, err := os.Stat(archivePath)
	require.NoError(t, err)
	assert.NotZero(t, info.Size())

	// 3. Open into a fresh directory.
	target := filepath.Join(t.TempDir(), "extracted")
	open := app.NewOpenCmd()
	open.SetOut(&buf)
	open.SetErr(&buf)
	open.SetArgs([]string{archivePath, "--into", target})
	require.NoError(t, open.Execute())

	// Recipe came back.
	extractedRecipe := filepath.Join(target, "round.kapi")
	data, err := os.ReadFile(extractedRecipe)
	require.NoError(t, err)
	assert.Contains(t, string(data), "id: round")

	// Sidecar survived.
	sidecar, err := os.ReadFile(filepath.Join(target, ".kapi", "collections", "ui", "targets", "fr.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"h1":"bonjour"}`, string(sidecar))
}

func TestOpenCmd_rejectsDirectoryArgument(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	cmd := app.NewOpenCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{dir})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a .klz")
}
