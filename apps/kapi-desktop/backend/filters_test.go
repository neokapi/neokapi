package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func filtersTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	recipe := filepath.Join(dir, "proj.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: proj\n"), 0o644))
	app := &App{projects: map[string]*openProject{"t": {Path: recipe}}}
	return app, dir
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

	// Shared → committed file; local → gitignored file.
	assert.FileExists(t, filepath.Join(dir, ".kapi", "filters.json"))
	assert.FileExists(t, filepath.Join(dir, ".kapi", "filters.local.json"))
	gi, _ := os.ReadFile(filepath.Join(dir, ".kapi", ".gitignore"))
	assert.Contains(t, string(gi), "filters.local.json")

	// Deleting the active (local) filter clears the active selection.
	require.NoError(t, app.DeleteProjectFilter("t", local.ID))
	got = app.GetProjectFilters("t")
	assert.Empty(t, got.Active)
	require.Len(t, got.Filters, 1)
	assert.Equal(t, shared.ID, got.Filters[0].ID)
}

func TestProjectFilters_ScopeMoveDoesNotDuplicate(t *testing.T) {
	app, _ := filtersTestApp(t)

	f, err := app.SaveProjectFilter("t", ProjectFilter{Name: "X", Shared: false})
	require.NoError(t, err)
	// Re-save the same id as shared — it must move to the committed file, not dupe.
	f.Shared = true
	_, err = app.SaveProjectFilter("t", *f)
	require.NoError(t, err)

	got := app.GetProjectFilters("t")
	require.Len(t, got.Filters, 1)
	assert.True(t, got.Filters[0].Shared)
}

func TestProjectFilters_NoProjectIsEmpty(t *testing.T) {
	app := &App{projects: map[string]*openProject{"t": {Path: ""}}}
	assert.Empty(t, app.GetProjectFilters("t").Filters)
	_, err := app.SaveProjectFilter("t", ProjectFilter{Name: "x"})
	require.Error(t, err)
}
