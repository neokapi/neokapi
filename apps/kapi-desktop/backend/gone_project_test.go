package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The desktop remembers projects by recipe path across launches. When the path
// stops resolving, every surface downstream of it reads the recipe and fails,
// and the project home reads an unwalkable directory as an empty project and
// offers to scaffold a fresh recipe into the folder the user deleted (#2560).
// These tests pin the two doors that lead there shut.

// scaffoldProject writes a minimal recipe and returns its path.
func scaffoldProject(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(path, &project.KapiProject{Version: project.CurrentVersion}))
	return path
}

func TestOpenProjectRefusesAGoneRecipe(t *testing.T) {
	tests := []struct {
		name    string
		remove  func(t *testing.T, path string)
		wantErr string
	}{
		{
			name:    "folder deleted",
			remove:  func(t *testing.T, path string) { require.NoError(t, os.RemoveAll(filepath.Dir(path))) },
			wantErr: "no longer exists",
		},
		{
			name:    "recipe deleted, folder kept",
			remove:  func(t *testing.T, path string) { require.NoError(t, os.Remove(path)) },
			wantErr: "is missing from",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp()
			path := scaffoldProject(t, "KapiMart")
			tt.remove(t, path)

			tab, err := app.OpenProject(path)
			require.Error(t, err)
			assert.Nil(t, tab)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, app.projects, "no tab is registered for a project that is not there")
		})
	}
}

// TestOpenProjectDoesNotHandBackAStaleTab is the case the walkthrough recorder
// hit: the app kept running while its home was wiped, so the tab from the
// previous pass was still in the map and matched by path. Reusing it restores a
// project with nothing behind it.
func TestOpenProjectDoesNotHandBackAStaleTab(t *testing.T) {
	app := NewApp()
	path := scaffoldProject(t, "KapiMart")

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	require.NotNil(t, tab)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	// The same recipe opened twice is one tab, while the recipe is there.
	again, err := app.OpenProject(path)
	require.NoError(t, err)
	assert.Equal(t, tab.ID, again.ID)

	require.NoError(t, os.RemoveAll(filepath.Dir(path)))

	_, err = app.OpenProject(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no longer exists")
}

// TestIsEmptyProjectOnAGoneDirectory: an unwalkable directory is not an empty
// project. Reporting it empty is what put the new-project template picker in
// front of a project the user had deleted.
func TestIsEmptyProjectOnAGoneDirectory(t *testing.T) {
	app := NewApp()
	path := scaffoldProject(t, "KapiMart")

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	assert.True(t, app.IsEmptyProject(tab.ID), "a folder holding only a recipe is empty")

	require.NoError(t, os.RemoveAll(filepath.Dir(path)))
	assert.False(t, app.IsEmptyProject(tab.ID), "a folder that is gone is not an empty project")
}

// TestGoneProjectIsRememberedNotRestored walks the whole session path: the
// project stays on the workspace home with its checkout marked missing, and
// stays out of the set the frontend reopens.
func TestGoneProjectIsRememberedNotRestored(t *testing.T) {
	app := newWorkspaceApp(t)
	app.settings = &settingsStore{
		filePath: filepath.Join(t.TempDir(), "settings.json"),
		settings: AppSettings{Theme: "system"},
	}

	kept := scaffoldProject(t, "Kept")
	gone := scaffoldProject(t, "KapiMart")

	for _, p := range []string{kept, gone} {
		tab, err := app.OpenProject(p)
		require.NoError(t, err)
		t.Cleanup(func() { app.CloseProject(tab.ID) })
	}
	app.SaveSessionState(SessionState{
		Mode:             AppModeProjects,
		LastOpenProjects: []string{gone, kept},
		ActiveProject:    gone,
	})

	require.NoError(t, os.RemoveAll(filepath.Dir(gone)))

	session := app.GetSessionState()
	assert.Equal(t, []string{kept}, session.LastOpenProjects)
	assert.Empty(t, session.ActiveProject)

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, home.Projects, 2, "a project whose folder went away keeps its place")
	missing := map[string]bool{}
	for _, p := range home.Projects {
		require.Len(t, p.Checkouts, 1)
		missing[p.Name] = p.Checkouts[0].Missing
	}
	assert.False(t, missing["Kept"])
	assert.True(t, missing["KapiMart"], "the checkout that went away is shown as missing")
}
