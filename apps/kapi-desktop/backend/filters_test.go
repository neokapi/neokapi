package backend

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
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

func getFilters(t *testing.T, app *App, tabID string) ProjectFilters {
	t.Helper()
	got, err := app.GetProjectFilters(tabID)
	require.NoError(t, err)
	return got
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

	got := getFilters(t, app, "t")
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
	got = getFilters(t, app, "t")
	assert.Empty(t, got.Active)
	require.Len(t, got.Filters, 1)
	assert.Equal(t, shared.ID, got.Filters[0].ID)

	// Deleting the shared one empties the project's set.
	require.NoError(t, app.DeleteProjectFilter("t", shared.ID))
	assert.Empty(t, getFilters(t, app, "t").Filters)
}

// A second checkout of the project gets the team's filters and none of the
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

	got := getFilters(t, app, "clone")
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

	got := getFilters(t, app, "t")
	require.Len(t, got.Filters, 1)
	assert.True(t, got.Filters[0].Shared)

	// And back to personal.
	f.Shared = false
	_, err = app.SaveProjectFilter("t", *f)
	require.NoError(t, err)
	got = getFilters(t, app, "t")
	require.Len(t, got.Filters, 1)
	assert.False(t, got.Filters[0].Shared)
}

func TestFilterEditPreservesBothStoresOnFailure(t *testing.T) {
	app, root := filtersTestApp(t)
	personal, err := app.SaveProjectFilter("t", ProjectFilter{Name: "Personal"})
	require.NoError(t, err)
	shared, err := app.SaveProjectFilter("t", ProjectFilter{Name: "Shared", Shared: true})
	require.NoError(t, err)
	require.NoError(t, app.SetActiveFilter("t", shared.ID))
	before := getFilters(t, app, "t")
	db, err := app.projectStore(app.getOpenProject("t"))
	require.NoError(t, err)
	_, err = db.Raw().ExecContext(t.Context(), `CREATE TRIGGER refuse_filter_edits BEFORE INSERT ON store_meta
		WHEN NEW.key = 'setting.saved-filters' BEGIN SELECT RAISE(ABORT, 'read-only setting'); END`)
	require.NoError(t, err)

	for _, filter := range []ProjectFilter{*personal, *shared} {
		filter.Shared = !filter.Shared
		filter.Name = "Changed"
		_, err := app.SaveProjectFilter("t", filter)
		require.ErrorContains(t, err, "read-only setting")
		assert.Equal(t, before, getFilters(t, app, "t"))
	}
	require.ErrorContains(t, app.DeleteProjectFilter("t", shared.ID), "read-only setting")
	assert.Equal(t, before, getFilters(t, app, "t"))
	_, err = db.Raw().ExecContext(t.Context(), "DROP TRIGGER refuse_filter_edits")
	require.NoError(t, err)

	ignore := filepath.Join(root, ".kapi", ".gitignore")
	require.NoError(t, os.Remove(ignore))
	require.NoError(t, os.Mkdir(ignore, 0o700))
	shared.Shared = false
	_, err = app.SaveProjectFilter("t", *shared)
	require.Error(t, err)
	assert.Equal(t, before, getFilters(t, app, "t"))
	require.Error(t, app.DeleteProjectFilter("t", personal.ID))
	assert.Equal(t, before, getFilters(t, app, "t"))
}

func TestFilterEditsPreserveUnreadablePreferences(t *testing.T) {
	app, root := filtersTestApp(t)
	f, err := app.SaveProjectFilter("t", ProjectFilter{Name: "Existing"})
	require.NoError(t, err)
	path := filepath.Join(root, ".kapi", "filters.local.json")
	broken := []byte("{broken")
	// The damaged file is kapi's own, so it is decoded rather than refused.
	require.NoError(t, project.WriteStateFile(path, broken, 0o600))

	_, err = app.GetProjectFilters("t")
	require.ErrorContains(t, err, "decode personal filters")
	_, err = app.SaveProjectFilter("t", *f)
	require.ErrorContains(t, err, "decode personal filters")
	require.ErrorContains(t, app.DeleteProjectFilter("t", f.ID), "decode personal filters")
	require.ErrorContains(t, app.SetActiveFilter("t", f.ID), "decode personal filters")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, broken, got)
}

func TestConcurrentFilterEditsAcrossCheckouts(t *testing.T) {
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	app, _ := filtersTestApp(t)
	first, err := project.Load(app.getOpenProject("t").Path)
	require.NoError(t, err)
	addFiltersCheckout(t, app, "clone", first.ID)
	const edits = 8
	var wg sync.WaitGroup
	for i := range edits {
		for _, tab := range []string{"t", "clone"} {
			wg.Go(func() {
				for _, shared := range []bool{true, false} {
					_, err := app.SaveProjectFilter(tab, ProjectFilter{Name: fmt.Sprintf("%s-%d-%t", tab, i, shared), Shared: shared})
					assert.NoError(t, err)
				}
			})
		}
	}
	wg.Wait()
	for _, tab := range []string{"t", "clone"} {
		assert.Len(t, getFilters(t, app, tab).Filters, 3*edits)
	}
}

func TestProjectFilter_MatchesFile(t *testing.T) {
	f := ProjectFilter{Collections: []string{"Website"}, Glob: "**/api*.md"}
	assert.True(t, f.MatchesFile("Website", "docs/api-reference.md"))
	assert.False(t, f.MatchesFile("", "docs/api-reference.md"), "no collection")
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
	assert.True(t, matchGlobPath("*.json", "en.json"))
	assert.True(t, matchGlobPath("src/**/*.json", "src/en.json"))
	assert.False(t, matchGlobPath("api.md", "docs/otherapi.md"))
	assert.False(t, matchGlobPath("**/api.md", "docs/otherapi.md"))
	assert.True(t, matchGlobPath("**/{api,guide}.[md][dx]", "api.md"))
	assert.False(t, matchGlobPath("[broken", "docs/[broken"))
}

func TestProjectFilters_NoProjectIsEmpty(t *testing.T) {
	app := &App{projects: map[string]*openProject{"t": {Path: ""}}}
	assert.Empty(t, getFilters(t, app, "t").Filters)
	_, err := app.SaveProjectFilter("t", ProjectFilter{Name: "x"})
	require.Error(t, err)
}
