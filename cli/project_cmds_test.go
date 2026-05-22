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

func TestInitCmd_framework(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := app.NewInitCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dir", dir, "--name", "demo", "--framework", "flutter", "--target-locale", "fr"})
	require.NoError(t, cmd.Execute())

	recipe := filepath.Join(dir, "demo.kapi")
	// The scaffolded recipe must parse and carry the framework's content mapping.
	p, err := project.Load(recipe)
	require.NoError(t, err)
	require.Len(t, p.Content, 1)

	items := p.Content[0].EffectiveItems()
	require.Len(t, items, 1)
	assert.Equal(t, "lib/l10n/app_en.arb", items[0].Path)
	require.NotNil(t, items[0].Format)
	assert.Equal(t, "json", items[0].Format.Name)
	assert.Equal(t, "lib/l10n/app_{lang}.arb", items[0].Target)
}

func TestInitCmd_frameworkKapiReactRejected(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := app.NewInitCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--dir", dir, "--framework", "kapi-react"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kapi-react")
	// No recipe should have been written.
	_, statErr := os.Stat(filepath.Join(dir, filepath.Base(dir)+".kapi"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestInitCmd_frameworkUnknown(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := app.NewInitCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--dir", dir, "--framework", "nope"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown framework")
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
