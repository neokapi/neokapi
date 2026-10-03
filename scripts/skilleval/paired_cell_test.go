package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The sandbox denies the checkout, the cell's private configuration and the
// attempt's records, and never the directories the shell and kapi run from.
func TestPairedClaudeDenyReadLeavesTheCellsProgramsReadable(t *testing.T) {
	attempt := t.TempDir()
	launch := PairedLaunch{StateDir: filepath.Join(attempt, "state"), Workspace: filepath.Join(attempt, "workspace"), RepoRoot: "/src/neokapi"}
	deny := pairedClaudeDenyRead(launch)
	assert.Contains(t, deny, "/src/neokapi")
	assert.Contains(t, deny, filepath.Join(attempt, "started.json"))
	assert.Contains(t, deny, filepath.Join(attempt, "prompt.txt"))
	assert.Contains(t, deny, filepath.Join(launch.StateDir, "claude-settings.json"))
	assert.NotContains(t, deny, filepath.Join(launch.StateDir, "claude"), "Claude reads its saved tool outputs from there")
	for _, path := range deny {
		for _, open := range []string{launch.StateDir, attempt, launch.Workspace} {
			assert.NotEqual(t, open, path, "denying %s would hide the cell's own programs", open)
		}
		assert.False(t, pairedWithin(filepath.Join(launch.StateDir, "bin", "zsh"), path), "%s covers the cell's bin", path)
		assert.False(t, pairedWithin(filepath.Join(launch.StateDir, "kapi", "kapi"), path), "%s covers the cell's kapi", path)
	}
}

// The cell is a repository with the project committed and a clean status, so
// `git diff` and `kapi check --diff-against HEAD` work as in a real project.
func TestPairedCellIsACleanRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	task := pairedTaskByID(t, "add-json-key")
	attempt := t.TempDir()
	launch := PairedLaunch{Workspace: filepath.Join(attempt, "workspace"), StateDir: filepath.Join(attempt, "state")}
	require.NoError(t, materializePairedTask(launch.Workspace, task))
	require.NoError(t, os.MkdirAll(filepath.Join(launch.Workspace, "kapi-data", "workspaces"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(launch.Workspace, "kapi-data", "workspaces", "store.db"), []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(launch.Workspace, ".claude", "skills", "kapi"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(launch.Workspace, ".claude", "skills", "kapi", "SKILL.md"), []byte("x"), 0o600))
	require.NoError(t, initPairedGit(t.Context(), launch))
	status := exec.Command("git", "status", "--porcelain")
	status.Dir = launch.Workspace
	status.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + attempt, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}
	output, err := status.Output()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(output)))
	files := exec.Command("git", "ls-files")
	files.Dir, files.Env = launch.Workspace, status.Env
	listed, err := files.Output()
	require.NoError(t, err)
	assert.Contains(t, string(listed), "locales/en.json")
	assert.NotContains(t, string(listed), "kapi-data")
	assert.NotContains(t, string(listed), ".claude")
	// The repository's own files are no content the task scopes.
	result, err := validatePairedTask(launch.Workspace, task, &PairedAgentResult{})
	require.NoError(t, err)
	assert.False(t, result.ObjectivePassed, "still nothing done")
	for _, criterion := range result.Criteria {
		if strings.HasPrefix(criterion.ID, "scope:") || strings.HasPrefix(criterion.ID, "unchanged:") {
			assert.True(t, criterion.Passed, criterion.ID)
		}
	}
}

func TestPairedKapiIsLinkedIntoTheCell(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "kapi")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700))
	launch := PairedLaunch{KapiBin: binary, StateDir: t.TempDir()}
	cell, err := linkPairedKapi(launch)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(launch.StateDir, "kapi", "kapi"), cell)
	same, err := os.ReadFile(cell)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\n", string(same))
	launch.CellKapi = cell
	assert.Equal(t, cell, launch.agentKapi())
	again, err := linkPairedKapi(launch)
	require.NoError(t, err)
	assert.Equal(t, cell, again)
}

// A cell's tools come from the system's directories, never through a version
// manager's shim in the developer's home directory.
func TestPairedCellToolsAreSystemTools(t *testing.T) {
	assert.NotEmpty(t, pairedSystemTool("sh"))
	assert.Empty(t, pairedSystemTool("no-such-tool-anywhere"))
	state := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(state, "bin"), 0o700))
	require.NoError(t, pairedToolPath(PairedLaunch{StateDir: state, Condition: "baseline"}))
	entries, err := os.ReadDir(filepath.Join(state, "bin"))
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(state, "bin", entry.Name()))
		require.NoError(t, err)
		assert.Contains(t, pairedSystemDirs, filepath.Dir(target), entry.Name())
	}
}

func TestPairedLocationRefusesClaudesTemporaryDirectory(t *testing.T) {
	dir := filepath.Join("/tmp", "claude-"+strconv.Itoa(os.Getuid()), "session", "study")
	err := checkPairedLocation(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Claude Code's temporary directory")
}
