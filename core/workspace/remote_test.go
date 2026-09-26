package workspace_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/core/workspace/workspacetest"
)

func TestFileRemoteConformance(t *testing.T) {
	workspacetest.RunRemoteConformance(t, func(t *testing.T) func(t *testing.T) workspace.Remote {
		share := filepath.Join(t.TempDir(), "share")
		require.NoError(t, os.Mkdir(share, 0o755))
		dir := filepath.Join(share, "project")
		return func(*testing.T) workspace.Remote { return workspace.NewFileRemote(dir) }
	})
}

func TestMemoryRemoteConformance(t *testing.T) {
	workspacetest.RunRemoteConformance(t, func(t *testing.T) func(t *testing.T) workspace.Remote {
		shared, err := workspace.NewMemoryRemote("context.kpz")
		require.NoError(t, err)
		return func(*testing.T) workspace.Remote { return shared }
	})
}

func TestFileRemoteReportsAMissingShare(t *testing.T) {
	r := workspace.NewFileRemote(filepath.Join(t.TempDir(), "not-mounted", "team", "project"))
	_, err := r.List(t.Context(), workspace.RemoteLogDir)
	require.ErrorIs(t, err, workspace.ErrRemoteUnreachable)
}

// git runs a git command for a test and returns its output.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return string(out)
}

// TestGitRemoteConformance runs the suite against a bare repository in a
// temporary directory, each handle a clone of it with a branch of its own
// commits, which is what a machine's checkout of a project is.
func TestGitRemoteConformance(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	workspacetest.RunRemoteConformance(t, func(t *testing.T) func(t *testing.T) workspace.Remote {
		root := t.TempDir()
		bare := filepath.Join(root, "origin.git")
		git(t, root, "init", "--quiet", "--bare", bare)
		seed := filepath.Join(root, "seed")
		git(t, root, "init", "--quiet", seed)
		require.NoError(t, os.WriteFile(filepath.Join(seed, "README.md"), []byte("docs\n"), 0o644))
		git(t, seed, "add", "README.md")
		git(t, seed, "commit", "--quiet", "-m", "start")
		git(t, seed, "remote", "add", "origin", bare)
		git(t, seed, "push", "--quiet", "origin", "HEAD:refs/heads/main")
		n := 0
		return func(t *testing.T) workspace.Remote {
			n++
			clone := filepath.Join(root, "clone"+string(rune('a'+n)))
			git(t, root, "clone", "--quiet", bare, clone)
			return workspace.NewGitRemote(clone, "", "")
		}
	})
}

func TestGitRemoteReportsAnUnreachableRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	git(t, root, "init", "--quiet", root)
	git(t, root, "remote", "add", "origin", filepath.Join(root, "missing.git"))
	r := workspace.NewGitRemote(root, "", "")
	_, err := r.List(t.Context(), workspace.RemoteLogDir)
	require.ErrorIs(t, err, workspace.ErrRemoteUnreachable)
}
