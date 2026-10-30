package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contentFolder writes a folder of documents with no recipe and returns it.
func contentFolder(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	files := map[string]string{
		"README.md":          "# Readme\n\nHello.\n",
		"docs/intro.md":      "# Intro\n\nFirst page.\n",
		"docs/guide/next.md": "# Next\n\nSecond page.\n",
	}
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return dir
}

// collectionPaths lists the path globs of a recipe's collections, in order.
func collectionPaths(proj *project.KapiProject) []string {
	out := []string{}
	for _, c := range proj.Collections {
		for _, item := range c.EffectiveItems() {
			out = append(out, item.Path)
		}
	}
	return out
}

// proposedPaths lists the globs `kapi init` proposes for dir.
func proposedPaths(t *testing.T, dir string) []string {
	t.Helper()
	e := &host.App{}
	e.InitRegistries()
	proposals, err := host.ProposeCollections(dir, host.ProposeOptions{Formats: e.FormatReg, SourceLocale: "en"})
	require.NoError(t, err)
	out := []string{}
	for _, p := range proposals {
		out = append(out, p.Path)
	}
	return out
}

// registeredDirs lists the checkouts the App's workspace records.
func registeredDirs(t *testing.T, app *App) []string {
	t.Helper()
	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	var out []string
	for _, p := range home.Projects {
		for _, c := range p.Checkouts {
			out = append(out, c.Path)
		}
	}
	return out
}

func TestOpenProjectOnAFolder(t *testing.T) {
	tests := []struct {
		name string
		// arrange returns the path handed to OpenProject and the folder it names.
		arrange func(t *testing.T, app *App) (path, dir string)
		// recipe is whether the folder held a recipe before it was opened.
		recipe bool
	}{
		{
			name: "folder without a recipe",
			arrange: func(t *testing.T, _ *App) (string, string) {
				dir := contentFolder(t, "Handbook")
				return dir, dir
			},
		},
		{
			name: "the kapi.yaml a folder does not hold yet",
			arrange: func(t *testing.T, _ *App) (string, string) {
				dir := contentFolder(t, "Handbook")
				return filepath.Join(dir, project.RecipeFileName), dir
			},
		},
		{
			name: "a file inside a folder without a recipe",
			arrange: func(t *testing.T, _ *App) (string, string) {
				dir := contentFolder(t, "Handbook")
				return filepath.Join(dir, "docs", "intro.md"), filepath.Join(dir, "docs")
			},
		},
		{
			name:   "folder with a recipe",
			recipe: true,
			arrange: func(t *testing.T, _ *App) (string, string) {
				path := scaffoldProject(t, "Handbook")
				return filepath.Dir(path), filepath.Dir(path)
			},
		},
		{
			name:   "a file inside a folder with a recipe",
			recipe: true,
			arrange: func(t *testing.T, _ *App) (string, string) {
				path := scaffoldProject(t, "Handbook")
				doc := filepath.Join(filepath.Dir(path), "notes.md")
				require.NoError(t, os.WriteFile(doc, []byte("# Notes\n"), 0o644))
				return doc, filepath.Dir(path)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newWorkspaceApp(t)
			path, dir := tt.arrange(t, app)

			tab, err := app.OpenProject(path)
			require.NoError(t, err)
			require.NotNil(t, tab)
			t.Cleanup(func() { app.CloseProject(tab.ID) })

			recipe := filepath.Join(dir, project.RecipeFileName)
			assert.Equal(t, recipe, tab.Path)
			assert.Equal(t, tt.recipe, tab.RecipeWritten)
			assert.Equal(t, filepath.Base(dir), tab.Name)

			if tt.recipe {
				assert.Contains(t, registeredDirs(t, app), host.NormalizeCheckoutPath(dir))
				return
			}
			assert.NoFileExists(t, recipe, "opening a folder writes no recipe")
			assert.NoDirExists(t, filepath.Join(dir, project.StateDirName), "opening a folder writes no state folder")
			assert.Empty(t, registeredDirs(t, app), "a folder is registered once its recipe is written")

			proj := app.GetProject(tab.ID)
			require.NotNil(t, proj)
			assert.Equal(t, filepath.Base(dir), proj.Name)
			assert.Equal(t, project.CurrentVersion, proj.Version)
			assert.Equal(t, "en", string(proj.Defaults.SourceLanguage))
			assert.NotEmpty(t, proj.ID)
			assert.Equal(t, proposedPaths(t, dir), collectionPaths(proj))
			assert.NotEmpty(t, collectionPaths(proj))

			again, err := app.OpenProject(dir)
			require.NoError(t, err)
			assert.Equal(t, tab.ID, again.ID, "the same folder opened twice is one tab")
		})
	}
}

func TestOpenProjectRefusesARegisteredFolderWithoutItsRecipe(t *testing.T) {
	app := newWorkspaceApp(t)
	path := scaffoldProject(t, "KapiMart")
	tab, err := app.OpenProject(filepath.Dir(path))
	require.NoError(t, err)
	app.CloseProject(tab.ID)
	require.NoError(t, os.Remove(path))

	for _, p := range []string{path, filepath.Dir(path)} {
		got, err := app.OpenProject(p)
		require.Error(t, err, p)
		assert.Nil(t, got)
		assert.Equal(t, recipeGoneError(path, recipeGoneDeleted).Error(), err.Error())
	}
	assert.Empty(t, app.projects)
}

func TestOpenProjectOnAFolderNamedLikeYAML(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := filepath.Join(t.TempDir(), "Docs: 2026 #draft")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	assert.Equal(t, "Docs: 2026 #draft", tab.Name)

	require.NoError(t, app.SaveProject(tab.ID))
	loaded, err := project.Load(filepath.Join(dir, project.RecipeFileName))
	require.NoError(t, err)
	assert.Equal(t, "Docs: 2026 #draft", loaded.Name)
}

func TestOpenProjectOnTheHomeFolderDoesNotScanIt(t *testing.T) {
	app := newWorkspaceApp(t)
	home := contentFolder(t, "home")
	t.Setenv("KAPI_HOME_DIR", home)

	tab, err := app.OpenProject(home)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	assert.False(t, tab.RecipeWritten)
	assert.Empty(t, app.GetProject(tab.ID).Collections, "a home folder is opened, not scanned")

	res, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	assert.False(t, res.Extracted)
	assert.NoDirExists(t, filepath.Join(home, project.StateDirName))
}

func TestEnsureRecipe(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := contentFolder(t, "Handbook")
	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	op := app.getOpenProject(tab.ID)
	recipe := filepath.Join(dir, project.RecipeFileName)

	require.NoError(t, app.ensureRecipe(op))
	require.FileExists(t, recipe)
	assert.True(t, app.ListTabs()[0].RecipeWritten)
	assert.Contains(t, registeredDirs(t, app), host.NormalizeCheckoutPath(dir))

	first, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(first), "kapi init proposed these", "the recipe keeps the comments kapi init writes")

	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, app.GetProject(tab.ID), loaded, "the written recipe is the one the tab holds")

	// A second call writes nothing.
	require.NoError(t, os.WriteFile(recipe, append(first, []byte("# edited by hand\n")...), 0o644))
	require.NoError(t, app.ensureRecipe(op))
	again, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(again), "# edited by hand")
}

func TestEnsureRecipeKeepsARecipeWrittenMeanwhile(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := contentFolder(t, "Handbook")
	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	recipe := filepath.Join(dir, project.RecipeFileName)
	theirs := "version: v1\nname: theirs\n"
	require.NoError(t, os.WriteFile(recipe, []byte(theirs), 0o644))

	require.NoError(t, app.ensureRecipe(app.getOpenProject(tab.ID)))
	got, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Equal(t, theirs, string(got))
}

// The recipe reaches the disk through the paths a person's change takes, and
// a sync that changes nothing writes nothing.
func TestUpdateProjectWritesTheRecipeOnAChange(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := contentFolder(t, "Handbook")
	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	recipe := filepath.Join(dir, project.RecipeFileName)

	// The frontend hands the recipe back through JSON on every rescan.
	same := roundTrip(t, app.GetProject(tab.ID))
	require.NoError(t, app.UpdateProject(tab.ID, same))
	assert.NoFileExists(t, recipe, "a sync with nothing changed writes no recipe")

	changed := roundTrip(t, app.GetProject(tab.ID))
	changed.Defaults.TargetLanguages = append(changed.Defaults.TargetLanguages, "fr")
	require.NoError(t, app.UpdateProject(tab.ID, changed))
	require.FileExists(t, recipe)

	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, []string{"fr"}, localeStrings(loaded.Defaults.TargetLanguages))
	assert.True(t, app.ListTabs()[0].RecipeWritten)
	assert.Contains(t, registeredDirs(t, app), host.NormalizeCheckoutPath(dir))
}

func TestSaveProjectWritesTheRecipeOfAFolder(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := contentFolder(t, "Handbook")
	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	require.NoError(t, app.SaveProject(tab.ID))
	recipe := filepath.Join(dir, project.RecipeFileName)
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(data), "kapi init proposed these")
	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, collectionPaths(app.GetProject(tab.ID)), collectionPaths(loaded))
}

func roundTrip(t *testing.T, proj *project.KapiProject) *project.KapiProject {
	t.Helper()
	data, err := json.Marshal(proj)
	require.NoError(t, err)
	var out project.KapiProject
	require.NoError(t, json.Unmarshal(data, &out))
	return &out
}

func localeStrings[T ~string](in []T) []string {
	out := make([]string, 0, len(in))
	for _, l := range in {
		out = append(out, string(l))
	}
	return out
}
