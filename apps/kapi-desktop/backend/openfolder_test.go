package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contentFolder writes a folder of documents with no recipe and returns it.
func contentFolder(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	files := map[string]string{
		"README.md":          "# Readme\n\nPlease sign in to continue.\n",
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

func TestEnsureRecipeRefusesTheHomeFolder(t *testing.T) {
	app := newWorkspaceApp(t)
	home := contentFolder(t, "home")
	t.Setenv("KAPI_HOME_DIR", home)

	tab, err := app.OpenProject(home)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	err = app.ensureRecipe(app.getOpenProject(tab.ID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a project folder")
	assert.Contains(t, err.Error(), "before you keep rules or connect an assistant")

	changed := roundTrip(t, app.GetProject(tab.ID))
	changed.Defaults.TargetLanguages = append(changed.Defaults.TargetLanguages, "fr")
	require.Error(t, app.UpdateProject(tab.ID, changed))
	require.Error(t, app.SaveProject(tab.ID))

	assert.NoFileExists(t, filepath.Join(home, project.RecipeFileName))
	assert.NoDirExists(t, filepath.Join(home, project.StateDirName))
	assert.False(t, app.ListTabs()[0].RecipeWritten)
	assert.Empty(t, registeredDirs(t, app))
}

func TestOpenProjectInsideAProjectOpensTheProject(t *testing.T) {
	app := newWorkspaceApp(t)
	recipe := scaffoldProject(t, "Handbook")
	root := filepath.Dir(recipe)
	sub := filepath.Join(root, "docs", "guide")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	doc := filepath.Join(sub, "next.md")
	require.NoError(t, os.WriteFile(doc, []byte("# Next\n"), 0o644))

	for _, path := range []string{sub, doc} {
		tab, err := app.OpenProject(path)
		require.NoError(t, err, path)
		assert.Equal(t, recipe, tab.Path, "%s opens the project it sits in", path)
		assert.True(t, tab.RecipeWritten)
		assert.Equal(t, "Handbook", tab.Name)
		assert.NoFileExists(t, filepath.Join(sub, project.RecipeFileName))
	}
	assert.Len(t, app.ListTabs(), 1, "the subfolder and the file are one project")
}

// A folder opened without a recipe is left as it was found: its block cache
// sits under the data root and the workspace records nothing, so it opens again
// as a folder. Writing the recipe moves the project into the folder, under the
// recipe's id alone, and keeps the context written before the write.
func TestAFolderStaysUntouchedUntilItsRecipeIsWritten(t *testing.T) {
	ctx := t.Context()
	app := newWorkspaceApp(t)
	dir := contentFolder(t, "Handbook")
	state := filepath.Join(dir, project.StateDirName)

	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	res, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	assert.True(t, res.Extracted, "the folder's documents are read into a cache")
	assert.FileExists(t, project.Layout{StateDir: host.DraftStateDir(dir)}.StorePath())
	assert.NoDirExists(t, state, "the cache is kept outside the folder")

	// Context written while the recipe is not on disk.
	before, err := app.hostEngine().Projector(ctx, dir)
	require.NoError(t, err)
	require.NoError(t, before.Terms().AddConcept(ctx, terms.Concept{
		ID:    "c-before",
		Terms: []terms.Term{{Text: "sign in", Locale: "en", Status: model.TermPreferred}},
	}))
	key := app.getOpenProject(tab.ID).workspaceKey
	app.CloseProject(tab.ID)

	again, err := app.OpenProject(dir)
	require.NoError(t, err, "a folder opened before opens again")
	t.Cleanup(func() { app.CloseProject(again.ID) })
	assert.False(t, again.RecipeWritten)
	assert.NoDirExists(t, state)
	assert.Empty(t, registeredDirs(t, app))
	assert.Equal(t, key, app.getOpenProject(again.ID).workspaceKey, "the draft keeps its id across opens")

	// Keeping a suggestion is what writes the recipe.
	require.NoError(t, app.ensureRecipeForKey(key))
	recipe := filepath.Join(dir, project.RecipeFileName)
	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, string(key), loaded.ID)
	assert.FileExists(t, filepath.Join(state, ".gitignore"))
	assert.FileExists(t, project.LayoutAt(dir).StorePath(), "the cache is rebuilt in the folder")

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, home.Projects, 1, "one registration")
	assert.Equal(t, string(key), home.Projects[0].Key)
	require.Len(t, home.Projects[0].Checkouts, 1)
	assert.False(t, home.Projects[0].Checkouts[0].Missing)

	// Context written after the recipe exists, through the path an agent takes.
	engine := agentProcess(t, app)
	proposal, err := engine.RecordContextObservation(ctx, host.ContextObserveRequest{
		Actor:     contextop.Actor{Kind: contextop.ActorAgent, Name: "claude@studio", Session: "sess-1"},
		Project:   recipe,
		Term:      "log in",
		InsteadOf: []string{"sign in"},
		Evidence:  []contextop.Evidence{{Path: "README.md", Unit: "readme/p", Quote: "Please sign in to continue."}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextSuggestion(ContextDecisionRequest{Project: string(key), ID: proposal.ID})
	require.NoError(t, err)

	after, err := app.hostEngine().Projector(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, key, after.Key())
	_, found, err := after.Terms().GetConcept(ctx, "c-before")
	require.NoError(t, err)
	assert.True(t, found, "context written before the recipe is still there")
	feed, err := app.ContextFeed(string(key), 0)
	require.NoError(t, err)
	require.NotEmpty(t, feed.Groups)
	assert.Equal(t, "established", feed.Groups[0].Entries[0].Status, "and the rule kept after it")
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

// The Map is the landing view of every tab, a folder opened without a recipe
// included: it reads the folder's draft cache.
func TestMapSummaryOfAFolderWithoutARecipe(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := contentFolder(t, "Handbook")
	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	_, err = app.SyncProjectStore(tab.ID)
	require.NoError(t, err)

	sum, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, sum.ReadFiles)
	assert.Positive(t, sum.Blocks)
	assert.NoDirExists(t, filepath.Join(dir, project.StateDirName))
}
