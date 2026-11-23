//go:build unix

package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A store handed out once is not handed out again after git starts tracking
// the state it lives in. Every hand-out, a cached handle included, asks again
// once the index changes. A long-lived host (the desktop, the MCP server) so
// sees a pull that brought state in at its next read.
func TestProjectDBAsksAgainAfterStateIsTracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q")
	a := &App{}
	t.Cleanup(a.Shutdown)
	_, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	// A repository that commits a store under .kapi/work.
	in := filepath.Join(root, project.StateDirName, project.WorkDirName, project.StoreFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(in), 0o755))
	require.NoError(t, os.WriteFile(in, []byte("committed"), 0o644))
	git("add", "-f", ".kapi/work/store.db")
	_, err = a.ProjectDB(t.Context(), root)
	require.ErrorContains(t, err, "git tracks .kapi/work")
}
