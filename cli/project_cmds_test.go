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

	// Recipe loads with source/target locales populated under defaults: —
	// the schema the loader actually reads (not top-level sourceLocale).
	p, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, "en", string(p.Defaults.SourceLanguage))
	var targets []string
	for _, l := range p.Defaults.TargetLanguages {
		targets = append(targets, string(l))
	}
	assert.Contains(t, targets, "fr")
}

func TestInitCmd_monolingual(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := app.NewInitCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dir", dir, "--name", "brand-site", "--monolingual"})
	require.NoError(t, cmd.Execute())

	// Recipe parses and validates.
	recipe := filepath.Join(dir, "brand-site.kapi")
	p, err := project.Load(recipe)
	require.NoError(t, err)

	// Monolingual: source language set, no target languages.
	assert.Equal(t, "en", string(p.Defaults.SourceLanguage))
	assert.Empty(t, p.Defaults.TargetLanguages)

	// Brand voice profile + termbase bound under defaults:.
	require.NotNil(t, p.Defaults.BrandVoice)
	assert.Equal(t, "professional-b2b", p.Defaults.BrandVoice.Pack)
	assert.Equal(t, ".kapi/termbase.db", p.Defaults.Termbase)

	// A check flow using the deterministic brand-vocabulary check.
	require.Contains(t, p.Flows, "check")
	require.NotNil(t, p.Flows["check"])
	steps := p.Flows["check"].Steps
	require.NotEmpty(t, steps)
	assert.Equal(t, "brand-vocab-check", steps[0].Tool)

	// State manifest still written (shared init scaffolding).
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	state, err := project.LoadState(layout)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "brand-site", state.Project.ID)
}

func TestInitCmd_monolingualRejectsTargetLocale(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := app.NewInitCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--dir", dir, "--monolingual", "--target-locale", "fr"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "monolingual")
	// No recipe should have been written.
	_, statErr := os.Stat(filepath.Join(dir, filepath.Base(dir)+".kapi"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestInitCmd_monolingualRejectsFramework(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := app.NewInitCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--dir", dir, "--monolingual", "--framework", "flutter"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "monolingual")
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

func TestInitCmd_idempotentOnExistingRecipe(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	// Pre-create the recipe file under the same name init will use.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.kapi"), []byte("version: v1\nname: existing\n"), 0o644))

	cmd := app.NewInitCmd()
	cmd.SetArgs([]string{"--dir", dir, "--name", "existing"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// Idempotent: re-running init on an existing project is not an error, so
	// plugin contributions (e.g. connecting to a server) can run on top of it.
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "already initialized")
}

func TestInitCmd_refusesDifferentNamedProject(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	// A project already exists under a different name.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.kapi"), []byte("version: v1\nname: existing\n"), 0o644))

	cmd := app.NewInitCmd()
	cmd.SetArgs([]string{"--dir", dir, "--name", "other"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already contains a kapi project")
}
