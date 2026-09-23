package backend

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// filtersTestApp opens one checkout of a project, "proj", in tab "t".
func filtersTestApp(t *testing.T) (*App, string) {
	t.Helper()
	isolateCheckPlugins(t)
	app := &App{
		projects: map[string]*openProject{},
		logger:   log.New(io.Discard, "", 0),
	}
	t.Cleanup(app.shutdownEngine)
	dir := addFiltersCheckout(t, app, "t", project.NewID())
	return app, dir
}

// addFiltersCheckout writes a checkout of the project with the given id and
// opens it in tab. Checkouts carrying one id share one context store.
func addFiltersCheckout(t *testing.T, app *App, tab, id string) string {
	t.Helper()
	dir := t.TempDir()
	recipe := filepath.Join(dir, project.RecipeFileName)
	body := "version: v1\nid: " + id + "\nname: proj\n"
	require.NoError(t, os.WriteFile(recipe, []byte(body), 0o644))
	app.projects[tab] = &openProject{Path: recipe}
	return dir
}

func TestProjectFilters_SharedAndLocalRoundTrip(t *testing.T) {
	app, dir := filtersTestApp(t)

	shared, err := app.SaveProjectFilter("t", ProjectFilter{
		Name: "DACH", Collections: []string{"Website"}, Languages: []string{"de-DE"}, Shared: true,
	})
	require.NoError(t, err)
	local, err := app.SaveProjectFilter("t", ProjectFilter{
		Name: "My FR", Languages: []string{"fr-FR"}, Shared: false,
	})
	require.NoError(t, err)

	require.NoError(t, app.SetActiveFilter("t", local.ID))

	got := app.GetProjectFilters("t")
	assert.Equal(t, local.ID, got.Active)
	require.Len(t, got.Filters, 2)

	byID := map[string]ProjectFilter{}
	for _, f := range got.Filters {
		byID[f.ID] = f
	}
	assert.True(t, byID[shared.ID].Shared, "shared filter flagged")
	assert.False(t, byID[local.ID].Shared, "local filter not shared")

	// The shared filter is a setting of the project, in its context store; the
	// personal one stays in this checkout, out of version control.
	db, err := app.projectStore(app.getOpenProject("t"))
	require.NoError(t, err)
	raw, ok, err := db.Setting(t.Context(), projectdb.SettingSavedFilters)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, raw, shared.ID)
	assert.NotContains(t, raw, local.ID)
	assert.NoFileExists(t, filepath.Join(dir, ".kapi", "filters.json"))
	assert.FileExists(t, filepath.Join(dir, ".kapi", "filters.local.json"))
	gi, _ := os.ReadFile(filepath.Join(dir, ".kapi", ".gitignore"))
	assert.True(t, project.GitignoreCovers(string(gi), project.LocalFiltersFilename), "rule: %q", gi)

	// Deleting the active (local) filter clears the active selection.
	require.NoError(t, app.DeleteProjectFilter("t", local.ID))
	got = app.GetProjectFilters("t")
	assert.Empty(t, got.Active)
	require.Len(t, got.Filters, 1)
	assert.Equal(t, shared.ID, got.Filters[0].ID)

	// Deleting the shared one empties the project's set.
	require.NoError(t, app.DeleteProjectFilter("t", shared.ID))
	assert.Empty(t, app.GetProjectFilters("t").Filters)
}

// A second checkout of the project sees the team's filters and none of the
// first checkout's personal ones.
func TestProjectFilters_SharedFiltersReachAnotherCheckout(t *testing.T) {
	// One data directory for both checkouts. Under test each checkout
	// otherwise gets a workspace of its own.
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	app, _ := filtersTestApp(t)
	shared, err := app.SaveProjectFilter("t", ProjectFilter{Name: "DACH", Languages: []string{"de-DE"}, Shared: true})
	require.NoError(t, err)
	_, err = app.SaveProjectFilter("t", ProjectFilter{Name: "Mine", Languages: []string{"fr-FR"}})
	require.NoError(t, err)

	first, err := project.Load(app.getOpenProject("t").Path)
	require.NoError(t, err)
	addFiltersCheckout(t, app, "clone", first.ID)
	_, err = app.projectStore(app.getOpenProject("clone"))
	require.NoError(t, err)

	got := app.GetProjectFilters("clone")
	require.Len(t, got.Filters, 1)
	assert.Equal(t, shared.ID, got.Filters[0].ID)
	assert.True(t, got.Filters[0].Shared)
}

func TestProjectFilters_ScopeMoveDoesNotDuplicate(t *testing.T) {
	app, _ := filtersTestApp(t)

	f, err := app.SaveProjectFilter("t", ProjectFilter{Name: "X", Shared: false})
	require.NoError(t, err)
	// Re-save the same id as shared: it moves to the project's set, not dupes.
	f.Shared = true
	_, err = app.SaveProjectFilter("t", *f)
	require.NoError(t, err)

	got := app.GetProjectFilters("t")
	require.Len(t, got.Filters, 1)
	assert.True(t, got.Filters[0].Shared)

	// And back to personal.
	f.Shared = false
	_, err = app.SaveProjectFilter("t", *f)
	require.NoError(t, err)
	got = app.GetProjectFilters("t")
	require.Len(t, got.Filters, 1)
	assert.False(t, got.Filters[0].Shared)
}

func TestProjectFilter_MatchesFile(t *testing.T) {
	f := ProjectFilter{Collections: []string{"Website"}, Glob: "**/api*.md"}
	assert.True(t, f.MatchesFile("Website", "docs/api-reference.md"))
	assert.False(t, f.MatchesFile("Store", "docs/api-reference.md"), "wrong collection")
	assert.False(t, f.MatchesFile("Website", "docs/guide.md"), "glob miss")

	// An empty filter narrows nothing.
	assert.False(t, ProjectFilter{}.FilesNarrowed())
	assert.True(t, ProjectFilter{}.MatchesFile("Any", "x/y.json"))
}

func TestMatchGlobPath(t *testing.T) {
	assert.True(t, matchGlobPath("*.json", "src/locales/en.json"), "bare glob matches anywhere")
	assert.False(t, matchGlobPath("src/*.json", "src/locales/en.json"), "* stays within a segment")
	assert.True(t, matchGlobPath("src/**/*.json", "src/locales/en.json"), "** crosses segments")
	assert.True(t, matchGlobPath("", "anything"), "empty glob matches all")
}

func TestProjectFilters_NoProjectIsEmpty(t *testing.T) {
	app := &App{projects: map[string]*openProject{"t": {Path: ""}}}
	assert.Empty(t, app.GetProjectFilters("t").Filters)
	_, err := app.SaveProjectFilter("t", ProjectFilter{Name: "x"})
	require.Error(t, err)
}
