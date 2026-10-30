package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
	"github.com/neokapi/neokapi/host"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The deeper look on the Map asks the demo provider, the one "Skip: use the
// demo engine" selects, which answers term-extract and voice-infer from the
// text itself. No test here reaches a real model.

// deeperLookApp starts an app over its own config and data directories, with
// no provider key in the environment.
func deeperLookTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	for _, vars := range providerenv.Vars {
		for _, v := range vars {
			t.Setenv(v, "")
		}
	}
	t.Setenv("KAPI_AI_PROVIDER", "")
	t.Setenv("KAPI_AI_MODEL", "")
	aiprovider.SetDemoNoticeWriter(discard{})
	app := NewApp()
	t.Cleanup(func() { app.shutdownEngine() })
	return app
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// harborProject writes a project whose files name Harbor Help and the
// Dockyard and address the reader as you, and opens it.
func harborProject(t *testing.T, app *App) (tabID, recipe string) {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("kapi.yaml", "version: v1\nname: harbor\ndefaults:\n  source_language: en\ncollections:\n  - name: docs\n    content:\n      - path: \"docs/*.md\"\n")
	write("docs/a.md", "# Harbor Help\n\nHarbor Help answers your questions. You can ask it anything.\n\n"+
		"Open the Dockyard to see your boats. You can't miss it.\n\n"+
		"The Dockyard lists every berth.\n\nYou reach Harbor Help from the Dockyard.\n")
	recipe = filepath.Join(dir, "kapi.yaml")
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID, recipe
}

func TestDeeperLook_NoModelConnected(t *testing.T) {
	app := deeperLookTestApp(t)
	tabID, _ := harborProject(t, app)

	model, err := app.DeeperLookModel(tabID)
	require.NoError(t, err)
	assert.False(t, model.Connected, "nothing names a model, so the Map offers to connect one")

	_, err = app.DeeperLook(tabID)
	require.Error(t, err)
	require.ErrorIs(t, err, host.ErrNoAIProvider)
}

func TestDeeperLook_FilesWhatTheMapShows(t *testing.T) {
	app := deeperLookTestApp(t)
	require.NoError(t, app.SelectAIProvider("demo", ""))
	tabID, _ := harborProject(t, app)

	model, err := app.DeeperLookModel(tabID)
	require.NoError(t, err)
	assert.Equal(t, &DeeperLookModelDTO{Connected: true, Provider: "demo", Model: aiprovider.DemoModelName}, model)

	before, err := app.MapSummary(tabID)
	require.NoError(t, err)
	assert.Zero(t, before.Suggested)

	got, err := app.DeeperLook(tabID)
	require.NoError(t, err)
	assert.Equal(t, "demo", got.Provider)
	assert.Equal(t, aiprovider.DemoModelName, got.Model)
	assert.Equal(t, 7, got.Filed, "two terms and five voice measures")
	assert.Equal(t, 5, got.Asked)
	assert.Equal(t, 5, got.Blocks)
	assert.Positive(t, got.Tokens)

	after, err := app.MapSummary(tabID)
	require.NoError(t, err)
	assert.Equal(t, 7, after.Suggested, "the new rows appear under What your files do")
	require.Len(t, after.Suggestions, 7)
	for _, row := range after.Suggestions {
		assert.Equal(t, "suggested", row.Group)
		assert.Equal(t, "tool", string(row.Item.NoticedBy.Kind), row.Item.ID)
		assert.Equal(t, aiprovider.DemoModelName, row.Item.NoticedBy.Name, "the queue names the model beside kapi")
		assert.Positive(t, row.Files, row.Item.ID)
	}

	again, err := app.DeeperLook(tabID)
	require.NoError(t, err)
	assert.Zero(t, again.Filed, "a second look files nothing new")
	settled, err := app.MapSummary(tabID)
	require.NoError(t, err)
	assert.Equal(t, 7, settled.Suggested)
}

func TestDeeperLook_WritesTheRecipeOfAFolderFirst(t *testing.T) {
	app := deeperLookTestApp(t)
	require.NoError(t, app.SelectAIProvider("demo", ""))
	dir := contentFolder(t, "harbor-docs")
	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	require.False(t, tab.RecipeWritten)

	_, err = app.DeeperLook(tab.ID)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err, "the run writes the folder's recipe, as running a flow does")
}
