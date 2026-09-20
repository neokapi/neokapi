package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitRepo builds a throwaway git repository with one commit and makes it the
// process working directory for the length of the test. The stream detector
// shells out to git in the current directory, so a fixture has to be a real
// checkout.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v: %s", args, out)
	}
	run("init", "--initial-branch=main")
	run("config", "user.email", "fixture@example.test")
	run("config", "user.name", "Fixture")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("one\n"), 0o644))
	run("add", "readme.txt")
	run("commit", "-m", "first")
	return dir
}

// clearDetection removes every environment variable the chain consults ahead of
// git, so a test naming one of them is the only one in force.
func clearDetection(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"BOWRAIN_STREAM",
		"GITHUB_ACTIONS", "GITHUB_EVENT_NAME", "GITHUB_HEAD_REF", "GITHUB_REF_NAME",
		"GITLAB_CI", "CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "CI_COMMIT_BRANCH",
		"CIRCLECI", "CIRCLE_BRANCH",
		"TF_BUILD", "SYSTEM_PULLREQUEST_SOURCEBRANCH", "BUILD_SOURCEBRANCHNAME",
		"JENKINS_URL", "CHANGE_BRANCH", "BRANCH_NAME", "GIT_BRANCH",
		"TRAVIS", "TRAVIS_PULL_REQUEST_BRANCH", "TRAVIS_BRANCH",
		"BUILDKITE", "BUILDKITE_BRANCH",
	} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}

// TestResolveStream_GitDetection covers the three answers git can give and the
// rule that CI is asked first. A branch is the stream; a directory that is not a
// checkout keeps the fallback; a detached HEAD refuses.
func TestResolveStream_GitDetection(t *testing.T) {
	t.Run("a branch is the stream", func(t *testing.T) {
		clearDetection(t)
		dir := gitRepo(t)
		cmd := exec.Command("git", "switch", "-c", "feature-x")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git switch: %s", out)
		t.Chdir(dir)

		stream, err := ResolveStream("", "")
		require.NoError(t, err)
		assert.Equal(t, "feature-x", stream)
	})

	t.Run("a directory that is no checkout falls back to main", func(t *testing.T) {
		clearDetection(t)
		t.Chdir(t.TempDir())

		stream, err := ResolveStream("", "")
		require.NoError(t, err)
		assert.Equal(t, "main", stream)
	})

	t.Run("a detached HEAD refuses", func(t *testing.T) {
		clearDetection(t)
		dir := gitRepo(t)
		cmd := exec.Command("git", "checkout", "--detach", "HEAD")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git checkout --detach: %s", out)
		t.Chdir(dir)

		_, err = ResolveStream("", "")
		require.ErrorIs(t, err, ErrDetachedHEAD)
		assert.Contains(t, err.Error(), "--stream")
		assert.Contains(t, err.Error(), "BOWRAIN_STREAM")
	})

	t.Run("CI wins over a detached checkout", func(t *testing.T) {
		clearDetection(t)
		dir := gitRepo(t)
		cmd := exec.Command("git", "checkout", "--detach", "HEAD")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git checkout --detach: %s", out)
		t.Chdir(dir)

		// A GitHub Actions checkout is detached by default, so the CI variables
		// have to answer before git is asked or every workflow run would refuse.
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_REF_NAME", "release-2")

		stream, err := ResolveStream("", "")
		require.NoError(t, err)
		assert.Equal(t, "release-2", stream)
	})
}

// TestResolveStream_ExplicitSourcesSkipGit: everything ahead of detection in the
// chain answers on its own, so a detached checkout is only ever a problem for a
// caller that named no stream anywhere.
func TestResolveStream_ExplicitSourcesSkipGit(t *testing.T) {
	cases := []struct {
		name   string
		flag   string
		env    string
		config string
		want   string
	}{
		{name: "the flag", flag: "from-flag", want: "from-flag"},
		{name: "the environment", env: "from-env", want: "from-env"},
		{name: "the recipe", config: "from-recipe", want: "from-recipe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearDetection(t)
			dir := gitRepo(t)
			cmd := exec.Command("git", "checkout", "--detach", "HEAD")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			require.NoErrorf(t, err, "git checkout --detach: %s", out)
			t.Chdir(dir)

			if tc.env != "" {
				t.Setenv("BOWRAIN_STREAM", tc.env)
			}
			stream, err := ResolveStream(tc.flag, tc.config)
			require.NoError(t, err)
			assert.Equal(t, tc.want, stream)
		})
	}
}
