package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pairedClaudeDenyRead lists what Claude's sandbox keeps from the agent: the
// checkout, the cell's private host configuration, and the attempt's own
// records beside the workspace (the prompt and the preparation, which names a
// task's interference). The cell's bin directory and its kapi stay readable
// without a carve-out from a denied directory, so the shell and kapi always
// execute.
func pairedClaudeDenyRead(launch PairedLaunch) []string {
	attempt := filepath.Dir(launch.StateDir)
	deny := []string{}
	if launch.RepoRoot != "" {
		deny = append(deny, launch.RepoRoot)
	}
	for _, name := range []string{"claude", "codex", "home", "claude-settings.json", "claude-mcp.json"} {
		deny = append(deny, filepath.Join(launch.StateDir, name))
	}
	for _, name := range []string{"prompt.txt", "preparation.json", "started.json", "result.json", "transcript.jsonl", "transcript.jsonl.stderr"} {
		deny = append(deny, filepath.Join(attempt, name))
	}
	return deny
}

// pairedGitExclude keeps the cell's runtime directories and the installed
// skill out of `git status`, so an agent starts from a clean tree.
var pairedGitExclude = []string{
	"/kapi-data/", "/kapi-config/", "/kapi-plugins/", "/xdg-data/", "/xdg-cache/",
	"/.claude/", "/.agents/", "/.kapi/work/",
}

// initPairedGit makes the workspace a repository with the project committed,
// as a project an agent works in is. Without one, the skill's own
// `kapi check --diff-against HEAD` and every agent's `git diff` fail for a
// reason no real project has. Author, committer and dates are fixed, and no
// configuration of the developer's is read.
func initPairedGit(ctx context.Context, launch PairedLaunch) error {
	git, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is needed to prepare a cell: %w", err)
	}
	run := func(args ...string) error {
		command := exec.CommandContext(ctx, git, args...)
		command.Dir = launch.Workspace
		command.Env = []string{
			"PATH=" + os.Getenv("PATH"), "HOME=" + filepath.Join(launch.StateDir, "home"),
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
			"GIT_AUTHOR_NAME=Harbor Help", "GIT_AUTHOR_EMAIL=docs@harbor.example",
			"GIT_COMMITTER_NAME=Harbor Help", "GIT_COMMITTER_EMAIL=docs@harbor.example",
			"GIT_AUTHOR_DATE=2026-09-01T09:00:00Z", "GIT_COMMITTER_DATE=2026-09-01T09:00:00Z",
		}
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w\n%s", args[0], err, output)
		}
		return nil
	}
	if err := run("init", "-q", "-b", "main"); err != nil {
		return err
	}
	exclude := filepath.Join(launch.Workspace, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(exclude, []byte(strings.Join(pairedGitExclude, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	if err := run("add", "-A"); err != nil {
		return err
	}
	return run("commit", "-q", "-m", "Harbor Help content")
}
