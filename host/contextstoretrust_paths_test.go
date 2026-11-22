//go:build !js && !windows

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// syncRefused syncs and requires the approval refusal.
func syncRefused(t *testing.T, app *App, recipe string) error {
	t.Helper()
	_, err := app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.Error(t, err)
	return err
}

// A folder inside the project that a pull replaces with a link to another
// folder is refused, and nothing is written where the link leads.
func TestContextStore_AFolderInTheProjectSwappedForALinkIsRefused(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return false }
	recipe := folderProject(t, app, "shared")
	root := filepath.Dir(recipe)
	require.NoError(t, os.Mkdir(filepath.Join(root, "shared"), 0o755))
	_, err := app.ApproveContextStore(recipe)
	require.NoError(t, err)

	outside := realTempDir(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, "shared")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "shared")))

	err = syncRefused(t, app, recipe)
	assert.Contains(t, err.Error(), "is a link")
	assert.NoDirExists(t, filepath.Join(outside, "log"), "nothing is written where the link leads")
	_, err = app.ApproveContextStore(recipe)
	require.Error(t, err, "a folder in the project reached through a link cannot be allowed either")
}

// An absolute folder whose parent is replaced by a link leads somewhere
// else, which asks again; the prompt and the approval name the folder the
// path leads to.
func TestContextStore_AParentSwappedForALinkAsksAgain(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return false }
	base := realTempDir(t)
	approved := filepath.Join(base, "share")
	require.NoError(t, os.Mkdir(approved, 0o755))
	recipe := folderProject(t, app, filepath.Join(base, "via", "team"))
	require.NoError(t, os.Symlink(approved, filepath.Join(base, "via")))

	location, err := ContextStoreLocation(recipe)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(approved, "team"), location, "the location shown is the resolved one")
	_, err = app.ApproveContextStore(recipe)
	require.NoError(t, err)
	_, err = app.SyncProjectContext(t.Context(), ContextSyncRequest{Project: recipe})
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(approved, "team"))

	elsewhere := realTempDir(t)
	require.NoError(t, os.Remove(filepath.Join(base, "via")))
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(base, "via")))
	assert.False(t, ContextStoreAllowed(recipe))
	err = syncRefused(t, app, recipe)
	require.ErrorIs(t, err, ErrContextStoreNotApproved)
	assert.NoDirExists(t, filepath.Join(elsewhere, "team"))
}

// The approval is bound to the project: another repository at the same path
// naming the same folder asks again.
func TestContextStore_AnotherProjectAtTheSamePathAsksAgain(t *testing.T) {
	app, _ := contextOpsApp(t)
	app.isTTY = func() bool { return false }
	folder := filepath.Join(realTempDir(t), "team")
	recipe := folderProject(t, app, folder)
	_, err := app.ApproveContextStore(recipe)
	require.NoError(t, err)
	require.True(t, ContextStoreAllowed(recipe))

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.ID = projectIDFor("someone-elses")
	require.NoError(t, project.Save(recipe, proj))

	assert.False(t, ContextStoreAllowed(recipe))
	require.ErrorIs(t, syncRefused(t, app, recipe), ErrContextStoreNotApproved)
}

// A path starting with ~ or naming an environment variable is refused, since
// kapi expands neither.
func TestContextStore_AHomeOrVariablePathIsRefused(t *testing.T) {
	app, _ := contextOpsApp(t)
	for _, path := range []string{"~/team", "$HOME/team", "${TEAM_SHARE}/kapi"} {
		t.Run(path, func(t *testing.T) {
			recipe := folderProject(t, app, path)
			err := syncRefused(t, app, recipe)
			assert.Contains(t, err.Error(), "kapi expands neither")
			assert.NoDirExists(t, filepath.Join(filepath.Dir(recipe), "~"))
		})
	}
}

// An approval the desktop sends names the folder its dialog showed; when the
// recipe leads elsewhere by the click, nothing is recorded.
func TestContextStore_AnApprovalOfAnotherFolderIsRefused(t *testing.T) {
	app, _ := contextOpsApp(t)
	shown := filepath.Join(realTempDir(t), "team")
	recipe := folderProject(t, app, shown)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Context.Path = filepath.Join(realTempDir(t), "other")
	require.NoError(t, project.Save(recipe, proj))

	_, err = app.ApproveContextStoreAt(recipe, shown)
	require.ErrorContains(t, err, "not the folder shown")
	assert.False(t, ContextStoreAllowed(recipe))
}
