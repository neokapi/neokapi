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

// trackedEntries are the entries of kapi's own state (project.StateEntries),
// each with the file a repository plants and commits there.
var trackedEntries = []struct{ entry, file string }{
	{project.WorkDirName, "work/store.db"},
	{project.SyncDirName, "sync/sync-cache.json"},
	{project.VaultDirName, "vault/originals.json"},
	{project.LocalFiltersFilename, project.LocalFiltersFilename},
}

func gitAt(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// trackedProject is a project in a git repository with file planted in its
// state directory, not tracked yet.
func trackedProject(t *testing.T, file string) (root, recipe string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root = t.TempDir()
	recipe = filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version: project.CurrentVersion, Name: "Tracked", Defaults: project.Defaults{SourceLanguage: "en"},
	}))
	p := filepath.Join(root, project.StateDirName, filepath.FromSlash(file))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(`{"active":"planted"}`), 0o644))
	gitAt(t, root, "init", "-q")
	return root, recipe
}

// Opening a project whose git tracks any entry of kapi's state is refused,
// naming the entry.
func TestOpenProjectRefusesTrackedState(t *testing.T) {
	for _, st := range trackedEntries {
		t.Run(st.entry, func(t *testing.T) {
			root, recipe := trackedProject(t, st.file)
			gitAt(t, root, "add", "-f", project.StateDirName+"/"+st.file)
			app := newTestApp(t)
			_, err := app.OpenProject(recipe)
			require.ErrorContains(t, err, "git tracks "+project.StateDirName+"/"+st.entry)
		})
	}
}

// A project open before its personal filters were tracked refuses the next
// read of them: the check asks git again once the repository's index changes.
func TestFiltersTrackedAfterOpenAreRefusedAtTheNextRead(t *testing.T) {
	root, recipe := trackedProject(t, project.LocalFiltersFilename)
	app := newTestApp(t)
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	_, err = app.GetProjectFilters(tab.ID)
	require.NoError(t, err)

	gitAt(t, root, "add", "-f", project.StateDirName+"/"+project.LocalFiltersFilename)
	got, err := app.GetProjectFilters(tab.ID)
	require.ErrorContains(t, err, "git tracks "+project.StateDirName+"/"+project.LocalFiltersFilename)
	assert.Empty(t, got.Active, "nothing planted is read")
}
