package backend

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// TestLaunchPath reads the project path a launch names: the first argument
// that is not a flag and exists, resolved against the launch's directory, and
// the recipe of a folder that holds one.
func TestLaunchPath(t *testing.T) {
	withRecipe := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(withRecipe, project.RecipeFileName), []byte("version: v1\nname: x\n"), 0o644))
	plain := t.TempDir()
	parent := filepath.Dir(plain)

	for _, tt := range []struct {
		name string
		args []string
		dir  string
		want string
	}{
		{name: "none", args: nil, want: ""},
		{name: "a folder with a recipe", args: []string{withRecipe}, want: filepath.Join(withRecipe, project.RecipeFileName)},
		{name: "a folder with none", args: []string{plain}, want: plain},
		{name: "the recipe itself", args: []string{filepath.Join(withRecipe, project.RecipeFileName)}, want: filepath.Join(withRecipe, project.RecipeFileName)},
		{name: "relative to the launch", args: []string{filepath.Base(plain)}, dir: parent, want: plain},
		{name: "flags are skipped", args: []string{"-psn_0_12345", "--verbose", plain}, want: plain},
		{name: "a path that does not exist", args: []string{filepath.Join(plain, "gone")}, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LaunchPath(tt.args, tt.dir))
		})
	}
}

// TestSingleInstanceID names the installation: the bundle identifier and a
// short digest of the executable's path.
func TestSingleInstanceID(t *testing.T) {
	id := SingleInstanceID()
	assert.True(t, strings.HasPrefix(id, bundleID+"."), id)
	assert.Equal(t, id, SingleInstanceID(), "one executable, one id")
}

// TestShareWritesWhatTheShareSectionWrites: `kapi share --via folder` and the
// desktop's Share section write the same `context:` block for the same folder.
func TestShareWritesWhatTheShareSectionWrites(t *testing.T) {
	app := newWorkspaceApp(t)
	tab, desktopRecipe := openSharingProject(t, app, "Desktop")
	_, cliRecipe := scaffoldNamedProject(t, filepath.Join(t.TempDir(), "cli"), project.NewID(), "Desktop")
	shared := filepath.Join(t.TempDir(), "team-share")

	_, err := app.SetProjectSharing(tab, project.ContextBackend{Backend: project.ContextBackendFile, Path: shared})
	require.NoError(t, err)
	_, err = app.hostEngine().Share(t.Context(), host.ShareRequest{Project: cliRecipe, Via: host.ShareViaFolder, Location: shared})
	require.NoError(t, err)

	fromDesktop, err := project.Load(desktopRecipe)
	require.NoError(t, err)
	fromCLI, err := project.Load(cliRecipe)
	require.NoError(t, err)
	assert.Equal(t, fromDesktop.Context, fromCLI.Context)

	block := func(path string) []byte {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		i := bytes.Index(data, []byte("context:"))
		require.GreaterOrEqual(t, i, 0, "%s names a place:\n%s", path, data)
		return data[i:]
	}
	assert.Equal(t, string(block(desktopRecipe)), string(block(cliRecipe)), "the same block, spelled the same way")
}
