//go:build unix

package backend

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A project whose state kapi on this machine did not write does not open; the
// desktop shows the state, and once the person vouches for it, it opens.
func TestOpenProjectAsksToVouchForStateThisMachineDidNotWrite(t *testing.T) {
	src := t.TempDir()
	srcRecipe := filepath.Join(src, project.RecipeFileName)
	require.NoError(t, project.Save(srcRecipe, &project.KapiProject{
		Version: project.CurrentVersion, Name: "Copied", Defaults: project.Defaults{SourceLanguage: "en"},
	}))
	require.NoError(t, project.EnsureLayout(project.LayoutAt(src)))
	require.NoError(t, os.MkdirAll(filepath.Join(src, ".kapi", "vault"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".kapi", "vault", "redaction.json"), []byte("{}"), 0o600))

	// The project copied elsewhere, as a restored backup or a copied folder.
	dst := filepath.Join(t.TempDir(), "copy")
	out, err := exec.Command("cp", "-R", src, dst).CombinedOutput()
	require.NoError(t, err, string(out))
	recipe := filepath.Join(dst, project.RecipeFileName)

	app := newTestApp(t)
	_, err = app.OpenProject(recipe)
	require.ErrorContains(t, err, "was not written by kapi on this machine")

	state, err := app.ProjectStateToVouch("")
	require.NoError(t, err)
	require.NotNil(t, state, "the project OpenProject refused is the one shown")
	assert.Equal(t, recipe, state.Path)
	assert.Contains(t, state.Entries, ".kapi/vault")
	assert.Contains(t, state.Origin, src, "where the state appears to come from")

	require.NoError(t, app.VouchForProjectState(state.Path))
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	again, err := app.ProjectStateToVouch(recipe)
	require.NoError(t, err)
	assert.Nil(t, again, "nothing is left to vouch for")
}

// The desktop records the state its dialog showed and nothing else: a project
// whose state was not shown, or changed after it was, is refused.
func TestVouchingInTheDesktopRecordsWhatTheDialogShowed(t *testing.T) {
	src := t.TempDir()
	srcRecipe := filepath.Join(src, project.RecipeFileName)
	require.NoError(t, project.Save(srcRecipe, &project.KapiProject{
		Version: project.CurrentVersion, Name: "Copied", Defaults: project.Defaults{SourceLanguage: "en"},
	}))
	require.NoError(t, project.EnsureLayout(project.LayoutAt(src)))
	require.NoError(t, project.WriteStateFile(filepath.Join(src, ".kapi", "vault", "redaction.json"), []byte("{}"), 0o600))
	dst := filepath.Join(t.TempDir(), "copy")
	out, err := exec.Command("cp", "-R", src, dst).CombinedOutput()
	require.NoError(t, err, string(out))
	recipe := filepath.Join(dst, project.RecipeFileName)

	app := newTestApp(t)
	require.ErrorContains(t, app.VouchForProjectState(recipe), "was not shown yet")

	state, err := app.ProjectStateToVouch(recipe)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.NoError(t, os.WriteFile(filepath.Join(dst, ".kapi", "vault", "added.json"), []byte("{}"), 0o600))
	require.ErrorContains(t, app.VouchForProjectState(recipe), "changed after it was shown")
	_, err = app.OpenProject(recipe)
	require.ErrorContains(t, err, "was not written by kapi on this machine", "nothing was recorded")
}
