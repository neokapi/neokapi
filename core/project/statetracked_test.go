//go:build unix

package project

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitIn runs git in dir with no global or system configuration.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// stateRepo is a project in a git repository whose state directory holds
// every entry of StateEntries, none of them tracked yet.
func stateRepo(t *testing.T) Layout {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
	l := LayoutAt(root)
	for _, e := range StateEntries {
		p := filepath.Join(l.StateDir, e)
		if e == LocalFiltersFilename {
			require.NoError(t, os.MkdirAll(l.StateDir, 0o755))
			require.NoError(t, os.WriteFile(p, []byte("{}"), 0o644))
			continue
		}
		require.NoError(t, os.MkdirAll(p, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(p, "kept"), []byte("planted"), 0o644))
	}
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", RecipeFileName)
	return l
}

// trackedPath is the path git tracks for a state entry.
func trackedPath(e string) string {
	if e == LocalFiltersFilename {
		return StateDirName + "/" + e
	}
	return StateDirName + "/" + e + "/kept"
}

// Each entry of kapi's own state that git tracks is refused, naming it, and
// untracking it clears the refusal without a restart: the check asks git
// again once the index changes.
func TestCheckStateTrackedRefusesEachTrackedEntry(t *testing.T) {
	for _, e := range StateEntries {
		t.Run(e, func(t *testing.T) {
			l := stateRepo(t)
			require.NoError(t, CheckStateTracked(t.Context(), l), "nothing tracked yet")
			gitIn(t, l.Root, "add", "-f", trackedPath(e))
			err := CheckStateTracked(t.Context(), l)
			require.ErrorContains(t, err, "git tracks "+StateDirName+"/"+e)
			assert.Contains(t, err.Error(), "git rm -r --cached")
			gitIn(t, l.Root, "rm", "-q", "-r", "--cached", trackedPath(e))
			require.NoError(t, CheckStateTracked(t.Context(), l), "untracking clears the refusal")
		})
	}
}

// A directory in no repository and a machine with no git pass; a git that
// fails, refuses to read the repository or does not answer in time, or no
// git at all, refuses with the cause and its remedy and never reads as "not
// tracked"; KAPI_TRUST_STATE=1 in the process environment vouches for the
// state; a cancelled caller context returns the cancellation.
func TestCheckStateTrackedOutcomes(t *testing.T) {
	t.Run("a directory in no repository passes", func(t *testing.T) {
		root := t.TempDir()
		l := LayoutAt(root)
		require.NoError(t, os.MkdirAll(l.WorkDir(), 0o755))
		require.NoError(t, CheckStateTracked(t.Context(), l))
	})

	t.Run("no git binary refuses, and the person can vouch", func(t *testing.T) {
		l := stateRepo(t)
		gitIn(t, l.Root, "add", "-f", trackedPath(WorkDirName))
		t.Setenv("PATH", t.TempDir())
		err := CheckStateTracked(t.Context(), l)
		require.ErrorIs(t, err, errGitUnanswered)
		assert.Contains(t, err.Error(), "git is not installed. Install git")
		assert.Contains(t, err.Error(), TrustStateEnv+"=1")
		t.Setenv(TrustStateEnv, "1")
		require.NoError(t, CheckStateTracked(t.Context(), l))
	})

	t.Run("a repository with no index yet is asked, and passes", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git is not installed")
		}
		root := t.TempDir()
		l := LayoutAt(root)
		require.NoError(t, os.MkdirAll(l.WorkDir(), 0o755))
		gitIn(t, root, "init", "-q")
		require.NoFileExists(t, filepath.Join(root, ".git", "index"))
		require.NoError(t, CheckStateTracked(t.Context(), l))
	})

	t.Run("a git that refuses a repository another user owns names the remedy", func(t *testing.T) {
		l := stateRepo(t)
		bin := t.TempDir()
		script := "#!/bin/sh\necho \"fatal: detected dubious ownership in repository at ''\" >&2\nexit 128\n"
		require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755))
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		err := CheckStateTracked(t.Context(), l)
		require.ErrorIs(t, err, errGitUnanswered)
		assert.Contains(t, err.Error(), "git config --global --add safe.directory "+l.Root)
		assert.NotContains(t, err.Error(), "Run again")
	})

	t.Run("a git that fails refuses", func(t *testing.T) {
		l := stateRepo(t)
		gitIn(t, l.Root, "add", "-f", trackedPath(WorkDirName))
		require.NoError(t, os.WriteFile(filepath.Join(l.Root, ".git", "index"), []byte("not an index"), 0o644))
		err := CheckStateTracked(t.Context(), l)
		require.ErrorIs(t, err, errGitUnanswered)
		assert.Contains(t, err.Error(), "could not ask git")
	})

	t.Run("a git that does not answer in time refuses", func(t *testing.T) {
		l := stateRepo(t)
		bin := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexec sleep 5\n"), 0o755))
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		prev := gitQueryTimeout
		gitQueryTimeout = 100 * time.Millisecond
		t.Cleanup(func() { gitQueryTimeout = prev })
		err := CheckStateTracked(t.Context(), l)
		require.ErrorIs(t, err, errGitUnanswered)
		assert.Contains(t, err.Error(), "no answer within")
	})

	t.Run("a cancelled caller context returns the cancellation", func(t *testing.T) {
		l := stateRepo(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := CheckStateTracked(ctx, l)
		require.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, errGitUnanswered, "a cancellation is not a refusal about git")
	})
}

// The query runs inside a checkout kapi does not trust, so the repository's
// own configuration runs nothing: a core.fsmonitor command that `git
// ls-files` would run is not run by the query.
func TestCheckStateTrackedRunsNoProgramTheRepositoryNames(t *testing.T) {
	l := stateRepo(t)
	gitIn(t, l.Root, "add", "-f", trackedPath(WorkDirName))
	marker := filepath.Join(t.TempDir(), "ran")
	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+marker+"\nexit 1\n"), 0o755))
	gitIn(t, l.Root, "config", "core.fsmonitor", hook)
	_ = exec.Command("git", "-C", l.Root, "update-index", "--fsmonitor").Run()
	_ = os.Remove(marker)

	// The repository's configuration does run the command on a plain query.
	plain := exec.Command("git", "-C", l.Root, "ls-files", "-z", "--", ".kapi/work")
	plain.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	_ = plain.Run()
	if _, err := os.Stat(marker); err != nil {
		t.Log("this git runs no fsmonitor command on ls-files; the query is checked anyway")
	}
	_ = os.Remove(marker)

	require.ErrorContains(t, CheckStateTracked(t.Context(), l), "git tracks")
	assert.NoFileExists(t, marker, "the query ran a program the repository named")
}
