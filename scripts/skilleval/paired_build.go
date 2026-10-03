package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// pairedBuild identifies the kapi binary a study runs.
type pairedBuild struct {
	Binary  string `json:"binary"`
	Version string `json:"version,omitempty"`
	Commit  string `json:"commit,omitempty"`
	Head    string `json:"head,omitempty"`
}

// checkPairedBuild refuses a study whose bin/kapi is missing or was built from
// another commit than the checkout's HEAD. The cells run that binary by its
// path; a kapi resolved from PATH is whatever release the developer installed
// last, so the study never reaches for one.
func checkPairedBuild(ctx context.Context, repoRoot string) (pairedBuild, error) {
	build := pairedBuild{Binary: findKapi(repoRoot)}
	if build.Binary == "" {
		return build, errors.New("bin/kapi is missing: the study runs the kapi built from this tree, by path; run make build")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	scratch, err := os.MkdirTemp("", "kapi-paired-build-")
	if err != nil {
		return build, err
	}
	defer os.RemoveAll(scratch)
	command := exec.CommandContext(ctx, build.Binary, "version", "--json")
	command.Dir = scratch
	command.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + scratch}, isolationEnv(scratch)...)
	output, err := command.Output()
	if err != nil {
		return build, fmt.Errorf("bin/kapi version: %w", err)
	}
	var reported struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal(output, &reported); err != nil {
		return build, fmt.Errorf("bin/kapi version: %w", err)
	}
	build.Version, build.Commit = reported.Version, reported.Commit
	head, err := exec.CommandContext(ctx, "git", "-C", repoRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		// Outside a git checkout there is no commit to compare.
		return build, nil
	}
	build.Head = strings.TrimSpace(string(head))
	if build.Commit == "" || build.Commit == "unknown" || !strings.HasPrefix(build.Head, build.Commit) {
		return build, fmt.Errorf("bin/kapi was built from %q but the checkout is at %s; run make build", build.Commit, build.Head)
	}
	return build, nil
}
