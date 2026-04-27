package project

import (
	"os"
	"os/exec"
	"strings"

	"github.com/neokapi/neokapi/bowrain/plugin/schema"
)

// ResolveStream determines the active stream name using the resolution chain:
//
//  1. flagValue (e.g. --stream)
//  2. BOWRAIN_STREAM environment variable
//  3. configStream (recipe's server.stream field), unless empty or $auto
//  4. CI / git branch auto-detection
//  5. "main" fallback
func ResolveStream(flagValue string, configStream string) string {
	if flagValue != "" {
		return schema.NormalizeStreamName(flagValue)
	}
	if env := os.Getenv("BOWRAIN_STREAM"); env != "" {
		return schema.NormalizeStreamName(env)
	}
	if configStream != "" && configStream != schema.StreamAuto {
		return schema.NormalizeStreamName(configStream)
	}
	if name := detectStreamFromCI(); name != "" {
		return schema.NormalizeStreamName(name)
	}
	if name := detectStreamFromGit(); name != "" {
		return schema.NormalizeStreamName(name)
	}
	return schema.StreamMain
}

// detectStreamFromCI returns the active branch from any recognized CI provider.
func detectStreamFromCI() string {
	if os.Getenv("GITHUB_ACTIONS") != "" {
		if os.Getenv("GITHUB_EVENT_NAME") == "pull_request" {
			if ref := os.Getenv("GITHUB_HEAD_REF"); ref != "" {
				return ref
			}
		}
		return os.Getenv("GITHUB_REF_NAME")
	}
	if os.Getenv("GITLAB_CI") != "" {
		if branch := os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"); branch != "" {
			return branch
		}
		return os.Getenv("CI_COMMIT_BRANCH")
	}
	if os.Getenv("CIRCLECI") != "" {
		return os.Getenv("CIRCLE_BRANCH")
	}
	if os.Getenv("TF_BUILD") != "" {
		if branch := os.Getenv("SYSTEM_PULLREQUEST_SOURCEBRANCH"); branch != "" {
			return branch
		}
		return os.Getenv("BUILD_SOURCEBRANCHNAME")
	}
	if os.Getenv("JENKINS_URL") != "" {
		if branch := os.Getenv("CHANGE_BRANCH"); branch != "" {
			return branch
		}
		if branch := os.Getenv("BRANCH_NAME"); branch != "" {
			return branch
		}
		return os.Getenv("GIT_BRANCH")
	}
	if os.Getenv("TRAVIS") != "" {
		if branch := os.Getenv("TRAVIS_PULL_REQUEST_BRANCH"); branch != "" {
			return branch
		}
		return os.Getenv("TRAVIS_BRANCH")
	}
	if os.Getenv("BUILDKITE") != "" {
		return os.Getenv("BUILDKITE_BRANCH")
	}
	return ""
}

// detectStreamFromGit returns the current git branch, or empty when detached.
func detectStreamFromGit() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output() //nolint:noctx // one-shot git query, no request context
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return ""
	}
	return branch
}
