package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeStreamName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"main", "main"},
		{"master", "main"},
		{"feature/new-ui", "feature/new-ui"},
		{"refs/heads/main", "main"},
		{"refs/heads/feature/foo", "feature/foo"},
		{"refs/tags/v1.0", "v1.0"},
		{"  main  ", "main"},
		{"", "main"},
		{"v2.0", "v2.0"},
		{"pr/142", "pr/142"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeStreamName(tt.input))
		})
	}
}

func TestResolveStream_FlagTakesPriority(t *testing.T) {
	assert.Equal(t, "v2.0", ResolveStream("v2.0", "main"))
	assert.Equal(t, "v2.0", ResolveStream("v2.0", "$auto"))
}

func TestResolveStream_ConfigValue(t *testing.T) {
	// Config with explicit stream (not $auto).
	result := ResolveStream("", "release/1.0")
	assert.Equal(t, "release/1.0", result)
}

func TestResolveStream_AutoFallsToGitOrMain(t *testing.T) {
	// With $auto config, it should try git then fall back to main.
	// In test env, git should work (we're in a git repo).
	result := ResolveStream("", "$auto")
	assert.NotEmpty(t, result, "should resolve to something")
}

func TestResolveStream_EmptyConfigIsAuto(t *testing.T) {
	// Empty config = same as $auto.
	result := ResolveStream("", "")
	assert.NotEmpty(t, result)
}

func TestResolveStream_EnvVar(t *testing.T) {
	t.Setenv("BOWRAIN_STREAM", "ci/deploy")
	assert.Equal(t, "ci/deploy", ResolveStream("", ""))
}

func TestResolveStream_FlagOverridesEnv(t *testing.T) {
	t.Setenv("BOWRAIN_STREAM", "ci/deploy")
	assert.Equal(t, "hotfix", ResolveStream("hotfix", ""))
}

// clearCIEnv unsets all CI detection env vars so tests start from a clean state.
// This is critical because CI runners (e.g. GitHub Actions) set their own env vars
// which would interfere with tests for other CI systems.
func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GITHUB_ACTIONS", "GITHUB_EVENT_NAME", "GITHUB_REF_NAME", "GITHUB_HEAD_REF",
		"GITLAB_CI", "CI_COMMIT_BRANCH", "CI_MERGE_REQUEST_SOURCE_BRANCH_NAME",
		"CIRCLECI", "CIRCLE_BRANCH",
		"TF_BUILD", "BUILD_SOURCEBRANCHNAME", "SYSTEM_PULLREQUEST_SOURCEBRANCH",
		"JENKINS_URL", "BRANCH_NAME", "CHANGE_BRANCH", "GIT_BRANCH",
		"TRAVIS", "TRAVIS_BRANCH", "TRAVIS_PULL_REQUEST_BRANCH",
		"BUILDKITE", "BUILDKITE_BRANCH",
	} {
		t.Setenv(key, "")
	}
}

func TestDetectStreamFromCI_GitHub(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_REF_NAME", "feature/foo")
	assert.Equal(t, "feature/foo", detectStreamFromCI())
}

func TestDetectStreamFromCI_GitHubPR(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_HEAD_REF", "feature/bar")
	t.Setenv("GITHUB_REF_NAME", "123/merge")
	assert.Equal(t, "feature/bar", detectStreamFromCI())
}

func TestDetectStreamFromCI_GitLab(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITLAB_CI", "true")
	t.Setenv("CI_COMMIT_BRANCH", "develop")
	assert.Equal(t, "develop", detectStreamFromCI())
}

func TestDetectStreamFromCI_GitLabMR(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("GITLAB_CI", "true")
	t.Setenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME", "feature/mr")
	t.Setenv("CI_COMMIT_BRANCH", "develop")
	assert.Equal(t, "feature/mr", detectStreamFromCI())
}

func TestDetectStreamFromCI_CircleCI(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("CIRCLECI", "true")
	t.Setenv("CIRCLE_BRANCH", "release/2.0")
	assert.Equal(t, "release/2.0", detectStreamFromCI())
}

func TestDetectStreamFromCI_AzureDevOps(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("TF_BUILD", "True")
	t.Setenv("BUILD_SOURCEBRANCHNAME", "main")
	assert.Equal(t, "main", detectStreamFromCI())
}

func TestDetectStreamFromCI_AzureDevOpsPR(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("TF_BUILD", "True")
	t.Setenv("SYSTEM_PULLREQUEST_SOURCEBRANCH", "feature/pr")
	t.Setenv("BUILD_SOURCEBRANCHNAME", "main")
	assert.Equal(t, "feature/pr", detectStreamFromCI())
}

func TestDetectStreamFromCI_Jenkins(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("JENKINS_URL", "http://jenkins.example.com")
	t.Setenv("BRANCH_NAME", "develop")
	assert.Equal(t, "develop", detectStreamFromCI())
}

func TestDetectStreamFromCI_JenkinsPR(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("JENKINS_URL", "http://jenkins.example.com")
	t.Setenv("CHANGE_BRANCH", "feature/pr-branch")
	t.Setenv("BRANCH_NAME", "PR-42")
	assert.Equal(t, "feature/pr-branch", detectStreamFromCI())
}

func TestDetectStreamFromCI_Travis(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("TRAVIS", "true")
	t.Setenv("TRAVIS_BRANCH", "main")
	assert.Equal(t, "main", detectStreamFromCI())
}

func TestDetectStreamFromCI_TravisPR(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("TRAVIS", "true")
	t.Setenv("TRAVIS_PULL_REQUEST_BRANCH", "feature/travis-pr")
	t.Setenv("TRAVIS_BRANCH", "main")
	assert.Equal(t, "feature/travis-pr", detectStreamFromCI())
}

func TestDetectStreamFromCI_Buildkite(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("BUILDKITE", "true")
	t.Setenv("BUILDKITE_BRANCH", "feature/bk")
	assert.Equal(t, "feature/bk", detectStreamFromCI())
}

func TestDetectStreamFromCI_NotCI(t *testing.T) {
	clearCIEnv(t)
	// No CI env vars set — should return empty.
	assert.Equal(t, "", detectStreamFromCI())
}

func TestDetectStreamFromGit(t *testing.T) {
	// We're in a git repo, so this should return a branch name or empty
	// (empty in detached HEAD, e.g. GitHub Actions PR merge refs).
	name := detectStreamFromGit()
	// Just verify it doesn't panic or error — the result depends on
	// the git state of the environment (branch vs detached HEAD).
	_ = name
}
