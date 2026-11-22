package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
)

// folderProject is a project whose recipe keeps its context in a folder the
// person did not choose here (as a cloned repository's recipe would), with
// one recorded change, so a sync that reached the folder would write to it.
func folderProject(t *testing.T, app *App, folder string) string {
	t.Helper()
	root := contextOpsProject(t, "folder-trust")
	proposeUtilise(t, app, root, person)
	proj, err := project.Load(recipeOf(root))
	require.NoError(t, err)
	proj.Context = &project.ContextBackend{Backend: project.ContextBackendFile, Path: folder}
	require.NoError(t, project.Save(recipeOf(root), proj))
	return recipeOf(root)
}

// realTempDir is a temporary directory named by its resolved path, so a
// comparison with the folder an approval resolves is not thrown off by a
// linked temporary root (macOS /var).
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

// The first sync through a folder the recipe names, with nobody to ask, is
// refused with the folder and the command that allows it, and nothing is
// read from or written to the folder.
func TestContextStore_FirstSyncIsRefusedWithoutAnApproval(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return false }
	folder := filepath.Join(t.TempDir(), "team")
	recipe := folderProject(t, app, folder)

	_, err := app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	var approval *ContextStoreApprovalError
	require.ErrorAs(t, err, &approval)
	assert.Equal(t, folder, approval.Location)
	assert.Contains(t, err.Error(), "kapi trust allow --context-store -p "+recipe)
	assert.Contains(t, err.Error(), "No terminal is attached")
	assert.NoDirExists(t, folder, "nothing is written to the folder before the approval")
}

// An approval is stored per user and per project: the next sync goes ahead
// and writes the log, and a recipe that names another folder asks again.
func TestContextStore_ApprovalIsStoredAndAChangedPathAsksAgain(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return false }
	folder := filepath.Join(realTempDir(t), "team")
	recipe := folderProject(t, app, folder)

	location, err := app.ApproveContextStore(recipe)
	require.NoError(t, err)
	assert.Equal(t, folder, location)
	assert.True(t, ContextStoreAllowed(recipe))
	_, err = app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.NoError(t, err)
	assert.DirExists(t, folder, "the sync wrote the project's log there")

	other := filepath.Join(t.TempDir(), "elsewhere")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Context.Path = other
	require.NoError(t, project.Save(recipe, proj))
	assert.False(t, ContextStoreAllowed(recipe))
	_, err = app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.ErrorIs(t, err, ErrContextStoreNotApproved)
	assert.NoDirExists(t, other)
}

// At a terminal the person is asked, with the folder shown, and the answer
// is recorded.
func TestContextStore_APersonAtATerminalIsAsked(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return true }
	folder := filepath.Join(t.TempDir(), "team")
	recipe := folderProject(t, app, folder)

	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString("y\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	stdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = stdin })

	_, err = app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.NoError(t, err)
	assert.True(t, ContextStoreAllowed(recipe), "the answer is recorded")
}

// An agent cannot allow a folder: not through the approval, not at a
// terminal, and not by naming it with kapi share --via folder.
func TestContextStore_AnAgentCannotApprove(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return true }
	app.ActAs(contextop.Actor{Kind: contextop.ActorAgent, Name: "claude"})
	folder := filepath.Join(t.TempDir(), "team")
	recipe := folderProject(t, app, folder)

	_, err := app.ApproveContextStore(recipe)
	require.ErrorContains(t, err, "only a person can allow")
	_, err = app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.ErrorContains(t, err, "An agent is running this command")
	assert.False(t, ContextStoreAllowed(recipe))

	_, err = app.Share(t.Context(), ShareRequest{Project: recipe, Via: ShareViaFolder, Location: folder})
	require.ErrorIs(t, err, ErrContextStoreNotApproved, "an agent naming the folder does not approve it")
	assert.NoDirExists(t, folder)
}

// An agent host's marker beside KAPI_ACTOR=person is still an agent, which
// cannot allow a folder.
func TestContextStore_AClaimedPersonCannotApprove(t *testing.T) {
	t.Run("an agent host claiming a person", func(t *testing.T) {
		t.Setenv("CLAUDECODE", "1")
		t.Setenv(EnvActor, "person")
		app, _ := contextOpsApp(t)
		recipe := folderProject(t, app, filepath.Join(t.TempDir(), "team"))
		_, err := app.ApproveContextStore(recipe)
		require.ErrorContains(t, err, "only a person can allow")
	})
}

// A person who names the folder with kapi share --via folder has chosen it.
func TestContextStore_SharingThroughAFolderApprovesIt(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contentFolder(t)
	folder := filepath.Join(t.TempDir(), "team")
	_, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaFolder, Location: folder})
	require.NoError(t, err)
	assert.True(t, ContextStoreAllowed(recipeOf(root)))
}
