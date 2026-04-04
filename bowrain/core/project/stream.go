package project

import (
	"os"
	"os/exec"
	"strings"
)

const (
	// StreamAuto is the sentinel value for auto-detecting the stream name.
	StreamAuto = "$auto"

	// StreamMain is the default stream name.
	StreamMain = "main"
)

// ResolveStream determines the active stream name using the resolution chain:
//
//  1. --stream flag (passed as flagValue; empty = not set)
//  2. BOWRAIN_STREAM env var
//  3. config.yaml stream field
//  4. $auto detection (git branch / CI heuristics)
//  5. "main" fallback
func ResolveStream(flagValue string, configStream string) string {
	// 1. Explicit flag.
	if flagValue != "" {
		return normalizeStreamName(flagValue)
	}

	// 2. Environment variable.
	if env := os.Getenv("BOWRAIN_STREAM"); env != "" {
		return normalizeStreamName(env)
	}

	// 3. Config value (unless $auto or empty).
	if configStream != "" && configStream != StreamAuto {
		return normalizeStreamName(configStream)
	}

	// 4. Auto-detect.
	if name := detectStreamFromCI(); name != "" {
		return normalizeStreamName(name)
	}
	if name := detectStreamFromGit(); name != "" {
		return normalizeStreamName(name)
	}

	// 5. Fallback.
	return StreamMain
}

// detectStreamFromCI checks CI environment variables to determine the branch name.
func detectStreamFromCI() string {
	// GitHub Actions.
	if os.Getenv("GITHUB_ACTIONS") != "" {
		if os.Getenv("GITHUB_EVENT_NAME") == "pull_request" {
			if ref := os.Getenv("GITHUB_HEAD_REF"); ref != "" {
				return ref
			}
		}
		return os.Getenv("GITHUB_REF_NAME")
	}

	// GitLab CI.
	if os.Getenv("GITLAB_CI") != "" {
		if branch := os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"); branch != "" {
			return branch
		}
		return os.Getenv("CI_COMMIT_BRANCH")
	}

	// CircleCI.
	if os.Getenv("CIRCLECI") != "" {
		return os.Getenv("CIRCLE_BRANCH")
	}

	// Azure DevOps.
	if os.Getenv("TF_BUILD") != "" {
		if branch := os.Getenv("SYSTEM_PULLREQUEST_SOURCEBRANCH"); branch != "" {
			return branch
		}
		return os.Getenv("BUILD_SOURCEBRANCHNAME")
	}

	// Jenkins.
	if os.Getenv("JENKINS_URL") != "" {
		if branch := os.Getenv("CHANGE_BRANCH"); branch != "" {
			return branch
		}
		if branch := os.Getenv("BRANCH_NAME"); branch != "" {
			return branch
		}
		return os.Getenv("GIT_BRANCH")
	}

	// Travis CI.
	if os.Getenv("TRAVIS") != "" {
		if branch := os.Getenv("TRAVIS_PULL_REQUEST_BRANCH"); branch != "" {
			return branch
		}
		return os.Getenv("TRAVIS_BRANCH")
	}

	// Buildkite.
	if os.Getenv("BUILDKITE") != "" {
		return os.Getenv("BUILDKITE_BRANCH")
	}

	return ""
}

// detectStreamFromGit shells out to git to get the current branch name.
func detectStreamFromGit() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output() //nolint:noctx // one-shot git query, no request context
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		// Detached HEAD — no meaningful branch.
		return ""
	}
	return branch
}

// normalizeStreamName cleans up a branch/ref name for use as a stream name.
func normalizeStreamName(name string) string {
	// Strip common ref prefixes.
	name = strings.TrimPrefix(name, "refs/heads/")
	name = strings.TrimPrefix(name, "refs/tags/")
	name = strings.TrimSpace(name)

	// Map main/master to the canonical "main" stream.
	if name == "master" {
		return StreamMain
	}

	if name == "" {
		return StreamMain
	}
	return name
}
