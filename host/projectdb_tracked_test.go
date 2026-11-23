//go:build unix

package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A store a repository commits under .kapi/work is not kapi's: the project
// store opens in the checkout's state directory under the data root, and the
// committed file is left as it was.
func TestProjectDBOpensNoStoreFromTheTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	in := filepath.Join(root, project.StateDirName, project.WorkDirName, project.StoreFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(in), 0o755))
	require.NoError(t, os.WriteFile(in, []byte("committed"), 0o644))
	for _, args := range [][]string{{"init", "-q"}, {"add", "-f", ".kapi/work/store.db"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	a := &App{}
	t.Cleanup(a.Shutdown)
	_, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	assert.FileExists(t, project.LayoutAt(root).StorePath(), "the store opens in the state directory")
	data, err := os.ReadFile(in)
	require.NoError(t, err)
	assert.Equal(t, "committed", string(data), "the committed file is not touched")
}
