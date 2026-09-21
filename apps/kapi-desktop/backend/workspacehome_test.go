package backend

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
)

// newWorkspaceApp builds an App on a workspace of its own, so one test's
// projects are never on another's home screen. A test binary already keeps out
// of the developer's data root; this keeps the tests out of each other's.
func newWorkspaceApp(t *testing.T) *App {
	t.Helper()
	app := NewApp()
	root := filepath.Join(t.TempDir(), "workspace")
	app.hostEngine().SetWorkspaceRoot(root)
	t.Cleanup(func() { app.shutdownEngine() })
	return app
}

// workspaceRootOf reports where an App's workspace is kept.
func workspaceRootOf(t *testing.T, app *App) string {
	t.Helper()
	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	return home.Location
}

// createTempKapi writes a bare recipe under a name of its own and returns its
// path, for a test that needs a file to point at rather than a project to open.
func createTempKapi(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".kapi")
	require.NoError(t, os.WriteFile(path, []byte("version: v1\nname: "+name), 0o644))
	return path
}

// scaffoldNamedProject writes a recipe carrying an id and a name, and returns
// the checkout directory and the recipe path.
func scaffoldNamedProject(t *testing.T, dir, id, name string) (string, string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(path, &project.KapiProject{
		Version: project.CurrentVersion,
		ID:      id,
		Name:    name,
	}))
	return dir, path
}

// TestOpenProjectRegistersItInTheWorkspace: "Open folder" is how a project the
// workspace has never seen joins it.
func TestOpenProjectRegistersItInTheWorkspace(t *testing.T) {
	app := newWorkspaceApp(t)

	before, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	assert.Empty(t, before.Projects, "a fresh workspace holds nothing")
	assert.NotEmpty(t, before.Location, "the home says where the workspace is")

	dir, recipe := scaffoldNamedProject(t, filepath.Join(t.TempDir(), "kapimart"), project.NewID(), "KapiMart")
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	after, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, after.Projects, 1)
	assert.Equal(t, "KapiMart", after.Projects[0].Name)
	assert.NotEmpty(t, after.Projects[0].LastActive)
	require.Len(t, after.Projects[0].Checkouts, 1)
	assert.Equal(t, host.NormalizeCheckoutPath(dir), after.Projects[0].Checkouts[0].Path)
	assert.False(t, after.Projects[0].Checkouts[0].Missing)
}

// TestWorktreesCollapseIntoOneProject: two checkouts of one repository share an
// id, so the home lists one project with two places to open it from.
func TestWorktreesCollapseIntoOneProject(t *testing.T) {
	app := newWorkspaceApp(t)
	id := project.NewID()
	base := t.TempDir()
	mainDir, mainRecipe := scaffoldNamedProject(t, filepath.Join(base, "kapimart"), id, "KapiMart")
	treeDir, treeRecipe := scaffoldNamedProject(t, filepath.Join(base, "kapimart-worktree"), id, "KapiMart")

	for _, recipe := range []string{mainRecipe, treeRecipe} {
		tab, err := app.OpenProject(recipe)
		require.NoError(t, err)
		t.Cleanup(func() { app.CloseProject(tab.ID) })
	}

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, home.Projects, 1, "one repository is one project, however many checkouts of it there are")
	paths := []string{}
	for _, c := range home.Projects[0].Checkouts {
		paths = append(paths, c.Path)
	}
	assert.ElementsMatch(t,
		[]string{host.NormalizeCheckoutPath(mainDir), host.NormalizeCheckoutPath(treeDir)}, paths)
}

// TestForgettingAProjectRemovesItAndItsContext: removal is explicit, takes the
// context store with it, and leaves the checkout on disk alone.
func TestForgettingAProjectRemovesItAndItsContext(t *testing.T) {
	app := newWorkspaceApp(t)
	id := project.NewID()
	dir, recipe := scaffoldNamedProject(t, filepath.Join(t.TempDir(), "kapimart"), id, "KapiMart")

	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	require.NotNil(t, tab)

	plan, err := app.WorkspaceRemovalFor(id)
	require.NoError(t, err)
	assert.Equal(t, "KapiMart", plan.Name)
	assert.Equal(t, []string{host.NormalizeCheckoutPath(dir)}, plan.Checkouts)
	assert.Equal(t,
		filepath.Join(workspaceRootOf(t, app), workspace.ProjectsDirName, id+".db"),
		plan.Store, "the confirmation names the file that will be deleted")

	// The store is there while the project is.
	ctx := t.Context()
	ws, err := app.hostEngine().Workspace(ctx)
	require.NoError(t, err)
	_, err = ws.Context(ctx, workspace.ProjectKey(id))
	require.NoError(t, err)
	require.FileExists(t, plan.Store)

	require.NoError(t, app.ForgetWorkspaceProject(id))

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	assert.Empty(t, home.Projects, "a removed project is off the home")
	assert.NoFileExists(t, plan.Store, "its context goes with it")
	assert.FileExists(t, recipe, "the checkout is untouched")
	assert.NotContains(t, app.projects, tab.ID, "the tab holding it is closed")
}

// TestClosingATabLeavesTheProjectRegistered: removal is never a side effect.
func TestClosingATabLeavesTheProjectRegistered(t *testing.T) {
	app := newWorkspaceApp(t)
	_, recipe := scaffoldNamedProject(t, filepath.Join(t.TempDir(), "kapimart"), project.NewID(), "KapiMart")

	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	app.CloseProject(tab.ID)

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, home.Projects, 1, "closing a tab is not removing a project")
}

// TestProjectWithNoCheckoutOpensOnItsContext: a project this machine has never
// had a copy of is still openable, on its context alone.
func TestProjectWithNoCheckoutOpensOnItsContext(t *testing.T) {
	app := newWorkspaceApp(t)
	ctx := t.Context()
	ws, err := app.hostEngine().Workspace(ctx)
	require.NoError(t, err)

	id := project.NewID()
	_, err = ws.Register(ctx, workspace.ProjectKey(id), "Elsewhere", "")
	require.NoError(t, err)

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, home.Projects, 1)
	assert.Empty(t, home.Projects[0].Checkouts, "no copy of it is on this machine")

	tab, err := app.OpenWorkspaceContext(id)
	require.NoError(t, err)
	require.NotNil(t, tab)
	assert.True(t, tab.ContextOnly)
	assert.Equal(t, "Elsewhere", tab.Name)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	handles := app.GetProjectHandles(tab.ID)
	assert.NotEmpty(t, handles.TermsHandle, "its terms are readable")
	assert.NotEmpty(t, handles.MemoryHandle, "its content memory is readable")

	// Asking for it again is the same tab rather than a second pool.
	again, err := app.OpenWorkspaceContext(id)
	require.NoError(t, err)
	assert.Equal(t, tab.ID, again.ID)
}

// TestWorkspaceWatcherSeesAnotherProcess is the live half: a second handle on
// the same workspace directory, standing in for the CLI or an agent's MCP
// server, registers a project and this app notices without being told.
func TestWorkspaceWatcherSeesAnotherProcess(t *testing.T) {
	app := newWorkspaceApp(t)
	root := workspaceRootOf(t, app)

	var (
		mu     sync.Mutex
		events int
	)
	InjectEventSink(app, func(name string, _ any) {
		if name != "workspace:changed" {
			return
		}
		mu.Lock()
		events++
		mu.Unlock()
	})

	watcher := newWorkspaceWatcher(app, 20*time.Millisecond)
	watcher.Start(t.Context())
	t.Cleanup(watcher.Stop)

	// The other process: its own backend on the same directory, exactly as a
	// `kapi` run in a terminal would open it.
	other, err := workspace.OpenLocal(t.Context(), root)
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })
	_, err = other.Register(t.Context(), "prj_fromtheterminal", "From the terminal", "/fakehome/src/elsewhere")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return events > 0
	}, 5*time.Second, 20*time.Millisecond, "the app notices a project registered by another process")

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	require.Len(t, home.Projects, 1)
	assert.Equal(t, "From the terminal", home.Projects[0].Name)
	require.Len(t, home.Projects[0].Checkouts, 1)
	assert.True(t, home.Projects[0].Checkouts[0].Missing,
		"a checkout that is not on this machine reads as missing")
}

// TestWorkspaceWatcherIsQuietWhenNothingChanges: an idle workspace produces no
// events, so the home is not refetching once a second for nothing.
func TestWorkspaceWatcherIsQuietWhenNothingChanges(t *testing.T) {
	app := newWorkspaceApp(t)
	_, recipe := scaffoldNamedProject(t, filepath.Join(t.TempDir(), "kapimart"), project.NewID(), "KapiMart")
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	var (
		mu     sync.Mutex
		events int
	)
	InjectEventSink(app, func(name string, _ any) {
		if name != "workspace:changed" {
			return
		}
		mu.Lock()
		events++
		mu.Unlock()
	})

	watcher := newWorkspaceWatcher(app, 10*time.Millisecond)
	// The first read arms the watcher at the current head without announcing
	// everything recorded before it.
	watcher.poll(t.Context(), false)
	for range 5 {
		assert.False(t, watcher.poll(t.Context(), true), "an unchanged log is not a change")
	}
	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, events)
}
