package host

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// standInBowrainRemote registers a bowrain backend that keeps the context in
// a folder, standing in for the server, and returns the folder.
func standInBowrainRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	RegisterContextRemote(project.ContextBackendBowrain, func(context.Context, project.ContextBackend) (workspace.Remote, error) {
		return workspace.NewFileRemote(dir), nil
	})
	t.Cleanup(func() {
		remoteOpenersMu.Lock()
		delete(remoteOpeners, project.ContextBackendBowrain)
		remoteOpenersMu.Unlock()
	})
	return dir
}

// A recipe connected to Bowrain with no context: block keeps its context on
// Bowrain.
func TestResolveContextBackend_ABowrainProjectKeepsItsContextThere(t *testing.T) {
	recipe := writeRecipe(t, `version: v1
source_language: en
bowrain:
  url: https://bowrain.example/acme/p123
`)
	info, err := ResolveContextBackend(recipe)
	require.NoError(t, err)
	assert.Equal(t, project.ContextBackendBowrain, info.Kind)
	assert.Equal(t, "venue", info.From)
	assert.Equal(t, "https://bowrain.example/acme/p123", info.Location)

	var b bytes.Buffer
	require.NoError(t, info.FormatText(&b))
	assert.Equal(t, "Context: Managed on Bowrain (https://bowrain.example/acme/p123), where the recipe connects the project.\n", b.String())
}

// A recipe connected to Bowrain that keeps its context anywhere else is
// refused with the command that moves it.
func TestResolveContextBackend_ASplitContextIsRefused(t *testing.T) {
	for _, tt := range []struct {
		name, block, where string
	}{
		{name: "git", block: "context:\n  backend: git\n", where: "git (origin refs/kapi/context)"},
		{name: "folder", block: "context:\n  backend: file\n  path: /srv/team\n", where: "file (/srv/team)"},
		{name: "this machine", block: "context:\n  backend: local\n", where: "this machine"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recipe := writeRecipe(t, "version: v1\nsource_language: en\n"+tt.block+"bowrain:\n  url: https://bowrain.example/acme/p123\n")
			_, err := ResolveContextBackend(recipe)
			var split *ContextSplitError
			require.True(t, errors.As(err, &split), "got %v", err)
			assert.Contains(t, err.Error(), "connects the project to Bowrain (https://bowrain.example/acme/p123)")
			assert.Contains(t, err.Error(), "keeps its context on "+tt.where+".")
			assert.Contains(t, err.Error(), "run `kapi share --via bowrain` to move it")
		})
	}
}

// Connecting a project shared through a folder moves its context to Bowrain:
// the recipe names the bowrain backend, and the sync that follows pushes the
// context there.
func TestShare_ViaBowrainMovesTheContext(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "move-folder")
	shared := filepath.Join(t.TempDir(), "team")
	_, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaFolder, Location: shared})
	require.NoError(t, err)
	proposeUtilise(t, app, root, person)
	res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Equal(t, 1, res.Sync.Push.Pushed, "the folder holds the change before the move")

	server := standInBowrainRemote(t)
	standInBowrain(t, app, `printf 'bowrain:\n  url: %s\n' "$4" >> kapi.yaml`)
	res, err = app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaBowrain, Location: "https://bowrain.example/acme/site"})
	require.NoError(t, err)

	require.NotNil(t, res.Moved)
	assert.Equal(t, ContextMove{From: project.ContextBackendFile, Location: shared}, *res.Moved)
	assert.Equal(t, []ShareFile{{Path: "kapi.yaml", Action: ShareFileUpdated}}, res.Files)
	proj, err := project.Load(recipeOf(root))
	require.NoError(t, err)
	assert.Equal(t, project.ContextBackend{Backend: project.ContextBackendBowrain}, *proj.Context)
	assert.Equal(t, ShareRungBowrain, res.Rung)
	require.NotNil(t, res.Sync)
	assert.Equal(t, 1, res.Sync.Push.Pushed, "the change reaches Bowrain")
	logs, err := workspace.NewFileRemote(server).List(t.Context(), workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.NotEmpty(t, logs)
	assert.Contains(t, shareText(t, res), "  context moved from file ("+shared+"), 0 operations read from there\n")
}

// A project already connected to Bowrain moves its context without naming
// the server again.
func TestShare_ViaBowrainMovesTheContextOfAConnectedProject(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "move-connected")
	recipe, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipeOf(root), append(recipe, []byte("context:\n  backend: local\nbowrain:\n  url: https://bowrain.example/acme/site\n")...), 0o600))
	proposeUtilise(t, app, root, person)

	standInBowrainRemote(t)
	standInBowrain(t, app, "exit 3") // the connect must not run
	res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaBowrain})
	require.NoError(t, err)
	assert.Equal(t, ContextMove{From: project.ContextBackendLocal}, *res.Moved)
	assert.Equal(t, 1, res.Sync.Push.Pushed)
	assert.Contains(t, shareText(t, res), "  context moved from this machine\n")
	info, err := ResolveContextBackend(recipeOf(root))
	require.NoError(t, err)
	assert.Equal(t, project.ContextBackendBowrain, info.Kind)
}

// A place the context cannot be read from stops the move before anything is
// written.
func TestShare_ViaBowrainStopsWhenTheContextCannotBeRead(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "move-unreadable")
	recipe, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)
	before := append(recipe, []byte("context:\n  backend: git\n")...)
	require.NoError(t, os.WriteFile(recipeOf(root), before, 0o600))
	proposeUtilise(t, app, root, person)

	standInBowrainRemote(t)
	standInBowrain(t, app, `printf 'bowrain:\n  url: %s\n' "$4" >> kapi.yaml`)
	_, err = app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaBowrain, Location: "https://bowrain.example/acme/site"})
	require.ErrorContains(t, err, "nothing was changed")
	after, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

// A project on Bowrain cannot be shared through another place as well, and
// the share's status names the split and its fix instead of failing.
func TestShare_ABowrainProjectRefusesASecondPlace(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "split")
	recipe, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)
	connected := append(recipe, []byte("bowrain:\n  url: https://bowrain.example/acme/site\n")...)
	require.NoError(t, os.WriteFile(recipeOf(root), connected, 0o600))

	_, err = app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Via: ShareViaFolder, Location: t.TempDir()})
	require.ErrorContains(t, err, "which keeps its context; it cannot be shared through file as well")
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	after, err := os.ReadFile(recipeOf(root))
	require.NoError(t, err)
	assert.Equal(t, string(connected), string(after))

	require.NoError(t, os.WriteFile(recipeOf(root), append(connected, []byte("context:\n  backend: git\n")...), 0o600))
	res, err := app.Share(t.Context(), ShareRequest{Project: recipeOf(root), Status: true})
	require.NoError(t, err)
	assert.Equal(t, ShareRungBowrain, res.Rung)
	assert.Contains(t, res.Problem, "run `kapi share --via bowrain` to move it")
	rung, err := RecipeRung(recipeOf(root))
	require.NoError(t, err)
	assert.Equal(t, ShareRungBowrain, rung)
}
