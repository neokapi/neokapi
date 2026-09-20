package project

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/neokapi/neokapi/host/venue/schema"
)

// ErrDetachedHEAD reports a checkout that names no branch, where the stream can
// only be stated rather than detected.
//
// A bisect, a tag checkout and a CI job that fetched one commit all sit on a
// detached HEAD. Reading that as the default stream sends the work of whatever
// commit is checked out to the stream the team's finished work lives on, and
// nothing in the run says so.
var ErrDetachedHEAD = errors.New(
	"this checkout is on a detached HEAD, so there is no branch to take the stream from: " +
		"pass --stream, or set BOWRAIN_STREAM, or name one under the recipe's server stream")

// ResolveStream determines the active stream name using the resolution chain:
//
//  1. flagValue (e.g. --stream)
//  2. BOWRAIN_STREAM environment variable
//  3. configStream (recipe's bowrain.stream field), unless empty or $auto
//  4. CI / git branch auto-detection
//  5. "main" fallback
//
// It returns ErrDetachedHEAD when the chain reaches git and git answers with a
// detached HEAD. A directory that is not a git checkout, and a machine with no
// git at all, keep the fallback: neither says anything about which stream the
// work belongs to, while a detached HEAD says the branch a caller would have
// taken it from does not exist.
func ResolveStream(flagValue string, configStream string) (string, error) {
	if flagValue != "" {
		return schema.NormalizeStreamName(flagValue), nil
	}
	if env := os.Getenv("BOWRAIN_STREAM"); env != "" {
		return schema.NormalizeStreamName(env), nil
	}
	if configStream != "" && configStream != schema.StreamAuto {
		return schema.NormalizeStreamName(configStream), nil
	}
	if name := detectStreamFromCI(); name != "" {
		return schema.NormalizeStreamName(name), nil
	}
	name, err := detectStreamFromGit()
	if err != nil {
		return "", err
	}
	if name != "" {
		return schema.NormalizeStreamName(name), nil
	}
	return schema.StreamMain, nil
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

// detectStreamFromGit returns the current git branch.
//
// Three answers, and they are three different facts. A branch name is the
// stream. An error from git is a directory that is not a checkout, or a machine
// with no git, and neither knows anything about streams, so the empty name
// hands the question back to the caller's fallback. The literal "HEAD" is a
// detached checkout, where git does know and the answer is that there is no
// branch.
func detectStreamFromGit() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output() //nolint:noctx // one-shot git query, no request context
	if err != nil {
		return "", nil //nolint:nilerr // not a checkout, or no git: the caller's fallback answers
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return "", ErrDetachedHEAD
	}
	return branch, nil
}
