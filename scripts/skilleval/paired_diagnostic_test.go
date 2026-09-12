package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPairedDiagnosticPromptAndSelection(t *testing.T) {
	opts := pairedTestOptions(t)
	m, err := readPairedManifest(opts.ManifestPath)
	require.NoError(t, err)
	schedule := pairedSchedule(m, "diagnostic")
	require.Len(t, schedule, 4)
	task, err := findPairedTask(m.SmokeTask)
	require.NoError(t, err)
	for _, session := range schedule {
		opts.Phase = "diagnostic"
		dir := t.TempDir()
		launch, err := materializePairedLaunch(opts, m, session, dir)
		require.NoError(t, err)
		assert.Contains(t, launch.Prompt, task.Prompt)
		assert.Contains(t, launch.Prompt, "explicit")
		assert.Contains(t, launch.Prompt, "unsupported")
		if session.Condition == "mcp" {
			assert.Contains(t, launch.Prompt, "context://content/en/page.json")
			assert.Contains(t, launch.Prompt, "no profile_file or profile_pack override")
		} else {
			assert.Contains(t, launch.Prompt, "Load the installed kapi skill")
		}
		saved, err := os.ReadFile(filepath.Join(dir, "prompt.txt"))
		require.NoError(t, err)
		assert.Equal(t, launch.Prompt+"\n", string(saved))
		opts.Phase = "smoke"
		natural, err := materializePairedLaunch(opts, m, session, t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, task.Prompt, natural.Prompt)
	}
	for _, selection := range []string{"unknown", schedule[0].ID + ",", schedule[0].ID + "," + schedule[0].ID} {
		_, err := selectPairedSessions(schedule, selection)
		require.Error(t, err)
	}
}

func TestPairedDiagnosticSharesCeilingAndRetainsPhase(t *testing.T) {
	opts := pairedTestOptions(t)
	opts.Phase = "diagnostic"
	opts.Sessions = "audience-child-claude-skill-cli-01,audience-child-codex-mcp-01"
	calls := 0
	deps := fakePairedDependencies(&calls)
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 2, calls)
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 2, calls)
	paths, err := pairedAttemptPaths(opts.Dir)
	require.NoError(t, err)
	require.Len(t, paths, 2)
	var study pairedStudyRecord
	require.NoError(t, readPairedJSON(filepath.Join(opts.Dir, "study.json"), &study))
	for _, path := range paths {
		row, err := scorePairedAttempt(path, study.Fingerprint)
		require.NoError(t, err)
		assert.Equal(t, "diagnostic", row.Phase)
	}
	opts.Phase = "smoke"
	opts.Sessions = ""
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 2, calls, "switching to natural tasks cannot reset the ceiling")
	opts.MaxAttempts = 3
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 3, calls)
}

func TestPairedDiagnosticOfflineSelection(t *testing.T) {
	opts := pairedTestOptions(t)
	opts.Phase = "diagnostic"
	opts.Live = false
	opts.Sessions = "audience-child-codex-mcp-01"
	calls := 0
	prepared := []PairedLaunch{}
	deps := fakePairedDependencies(&calls)
	deps.prepare = func(_ context.Context, launch PairedLaunch) (PairedPrepared, error) {
		prepared = append(prepared, launch)
		return PairedPrepared{Launch: launch, Version: "fixture", Blockers: []string{}}, nil
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Zero(t, calls)
	require.Len(t, prepared, 1)
	assert.Equal(t, "codex", prepared[0].Agent.Host)
	assert.Equal(t, "mcp", prepared[0].Condition)
	assert.Contains(t, prepared[0].Prompt, "explicit MCP integration diagnostic")
}
