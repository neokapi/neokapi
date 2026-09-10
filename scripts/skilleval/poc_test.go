package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pocTestOptions(t *testing.T) (pocOptions, pocStage) {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{"scripts/skilleval/runner.go", "cli/skills/data/kapi/SKILL.md", "bin/kapi"} {
		full := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o700))
		require.NoError(t, os.WriteFile(full, []byte("fixture "+path), 0o700))
	}
	workspace := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "brief.md"), []byte("Supplied authoring brief."), 0o600))
	private := t.TempDir()
	promptPath := filepath.Join(private, "prompt.txt")
	require.NoError(t, os.WriteFile(promptPath, []byte("Write the requested document using the supplied brief."), 0o600))
	stage := pocStage{ID: "draft", Workspace: workspace, PromptFile: promptPath, Condition: "baseline",
		TimeoutSeconds: 600, MaxTurns: 40, InputFiles: []string{"brief.md"}, ExpectedOutputs: []string{"draft.md"}}
	opts := pocOptions{StagePath: filepath.Join(private, "stage.json"), Dir: filepath.Join(private, "ledger"), RepoRoot: root}
	writePOCTestStage(t, opts.StagePath, stage)
	return opts, stage
}

func writePOCTestStage(t *testing.T, path string, stage pocStage) {
	t.Helper()
	data, err := json.Marshal(stage)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func TestPOCPreflightFreezesInputsWithoutInferenceOrReservation(t *testing.T) {
	opts, stage := pocTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	require.NoError(t, executePOCWith(context.Background(), opts, deps))
	assert.Zero(t, calls)
	dir := filepath.Join(opts.Dir, "stages", stage.ID)
	require.NoFileExists(t, filepath.Join(dir, "started.json"))
	var frozen pocFrozen
	require.NoError(t, readPairedJSON(filepath.Join(dir, "frozen.json"), &frozen))
	assert.NotEmpty(t, frozen.CodeHash)
	assert.Equal(t, meaningTextHash("Supplied authoring brief."), frozen.InputHashes["brief.md"])
	assert.FileExists(t, filepath.Join(dir, "inputs", "brief.md"))
	assert.FileExists(t, filepath.Join(dir, "prompt.txt"))
	count, limited, err := pocStartedCount(filepath.Join(opts.Dir, "stages"))
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.False(t, limited)
}

func TestPOCPreservesOriginalsOutputsAndFailedAttempts(t *testing.T) {
	opts, stage := pocTestOptions(t)
	opts.Live = true
	stage.ExpectedOutputs = []string{"brief.md", "missing.md"}
	writePOCTestStage(t, opts.StagePath, stage)
	calls := 0
	deps := fakePairedDependencies(&calls)
	deps.run = func(_ context.Context, p PairedPrepared) (PairedAgentResult, error) {
		calls++
		assert.Equal(t, pocAgent(), p.Launch.Agent)
		assert.Equal(t, "baseline", p.Launch.Condition)
		assert.False(t, p.Launch.NoTools)
		dir := filepath.Join(opts.Dir, "stages", stage.ID)
		require.FileExists(t, filepath.Join(dir, "started.json"), "reservation must precede inference")
		original, err := os.ReadFile(filepath.Join(dir, "inputs", "brief.md"))
		require.NoError(t, err)
		assert.Equal(t, "Supplied authoring brief.", string(original))
		require.NoError(t, os.WriteFile(filepath.Join(stage.Workspace, "brief.md"), []byte("Authored document."), 0o600))
		require.NoError(t, os.WriteFile(p.Launch.TranscriptPath, []byte("retained transcript"), 0o600))
		return PairedAgentResult{Status: "failed", FinalText: "Partial work retained.", Tools: []string{"Write"}}, errors.New("host interrupted")
	}
	require.ErrorContains(t, executePOCWith(context.Background(), opts, deps), "host interrupted")
	var result pocResult
	dir := filepath.Join(opts.Dir, "stages", stage.ID)
	require.NoError(t, readPairedJSON(filepath.Join(dir, "result.json"), &result))
	assert.Equal(t, "failed", result.Agent.Status)
	assert.Equal(t, "Partial work retained.", result.Agent.FinalText)
	require.Len(t, result.Outputs, 2)
	assert.Equal(t, meaningTextHash("Authored document."), result.Outputs[0].SHA256)
	assert.NotEmpty(t, result.Outputs[1].Error)
	retained, err := os.ReadFile(filepath.Join(dir, "outputs", "brief.md"))
	require.NoError(t, err)
	assert.Equal(t, "Authored document.", string(retained))
	assert.FileExists(t, filepath.Join(dir, "transcript.jsonl"))
	// Authored changes to overlapping inputs do not cause a rerun on resume.
	require.NoError(t, executePOCWith(context.Background(), opts, deps))
	assert.Equal(t, 1, calls)
}

func TestPOCChangedFrozenInputsBlockLaunch(t *testing.T) {
	for _, change := range []string{"prompt", "input", "runner", "skill", "binary"} {
		t.Run(change, func(t *testing.T) {
			opts, stage := pocTestOptions(t)
			stage.Condition, stage.KapiBin = "skill", filepath.Join(opts.RepoRoot, "bin", "kapi")
			writePOCTestStage(t, opts.StagePath, stage)
			calls := 0
			deps := fakePairedDependencies(&calls)
			require.NoError(t, executePOCWith(context.Background(), opts, deps))
			var path string
			switch change {
			case "prompt":
				path = stage.PromptFile
			case "input":
				path = filepath.Join(stage.Workspace, "brief.md")
			case "runner":
				path = filepath.Join(opts.RepoRoot, "scripts", "skilleval", "runner.go")
			case "skill":
				path = filepath.Join(opts.RepoRoot, "cli", "skills", "data", "kapi", "SKILL.md")
			case "binary":
				path = stage.KapiBin
			}
			require.NoError(t, os.WriteFile(path, []byte("Changed frozen input."), 0o700))
			opts.Live = true
			require.Error(t, executePOCWith(context.Background(), opts, deps))
			assert.Zero(t, calls)
			assert.NoFileExists(t, filepath.Join(opts.Dir, "stages", stage.ID, "started.json"))
		})
	}
}

func TestPOCSixStartedStagesIncludeFailuresAndInterruptedReservation(t *testing.T) {
	opts, stage := pocTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	require.NoError(t, executePOCWith(context.Background(), opts, deps))
	require.NoError(t, writePairedJSON(filepath.Join(opts.Dir, "stages", stage.ID, "started.json"), pocStarted{}))
	opts.Live = true
	require.NoError(t, executePOCWith(context.Background(), opts, deps))
	assert.Zero(t, calls, "a reserved interruption cannot be retried")
	deps.run = func(context.Context, PairedPrepared) (PairedAgentResult, error) {
		calls++
		return PairedAgentResult{Status: "failed", Tools: []string{}}, nil
	}
	for i := 1; i < pocAttemptLimit; i++ {
		stage.ID = fmt.Sprintf("stage-%d", i)
		writePOCTestStage(t, opts.StagePath, stage)
		require.NoError(t, executePOCWith(context.Background(), opts, deps))
	}
	assert.Equal(t, pocAttemptLimit-1, calls)
	stage.ID = "over-limit"
	writePOCTestStage(t, opts.StagePath, stage)
	require.ErrorContains(t, executePOCWith(context.Background(), opts, deps), "attempt limit")
	assert.Equal(t, pocAttemptLimit-1, calls)
}

func TestPOCSkillLaunchAndPreparationMutationGuard(t *testing.T) {
	opts, stage := pocTestOptions(t)
	stage.Condition, stage.KapiBin = "skill", filepath.Join(opts.RepoRoot, "bin", "kapi")
	writePOCTestStage(t, opts.StagePath, stage)
	opts.Live = true
	calls := 0
	deps := fakePairedDependencies(&calls)
	prepare := deps.prepare
	deps.prepare = func(ctx context.Context, launch PairedLaunch) (PairedPrepared, error) {
		assert.Equal(t, "skill-cli", launch.Condition)
		actualBinary, err := filepath.EvalSymlinks(stage.KapiBin)
		require.NoError(t, err)
		assert.Equal(t, actualBinary, launch.KapiBin)
		require.NoError(t, os.WriteFile(filepath.Join(stage.Workspace, "brief.md"), []byte("Unexpected preparation change."), 0o600))
		return prepare(ctx, launch)
	}
	require.ErrorContains(t, executePOCWith(context.Background(), opts, deps), "changed during preparation")
	assert.Zero(t, calls)
	assert.NoFileExists(t, filepath.Join(opts.Dir, "stages", stage.ID, "started.json"))
}

func TestPOCRateLimitPreventsFurtherStageLaunches(t *testing.T) {
	opts, stage := pocTestOptions(t)
	opts.Live = true
	calls := 0
	deps := fakePairedDependencies(&calls)
	deps.run = func(context.Context, PairedPrepared) (PairedAgentResult, error) {
		calls++
		return PairedAgentResult{Status: "rate_limited", RateLimited: true, Tools: []string{}}, nil
	}
	require.ErrorContains(t, executePOCWith(context.Background(), opts, deps), "rate limiting")
	stage.ID = "next-stage"
	writePOCTestStage(t, opts.StagePath, stage)
	require.ErrorContains(t, executePOCWith(context.Background(), opts, deps), "rate-limited attempt")
	assert.Equal(t, 1, calls)
}

func TestPOCRejectsUnsafePathsAndLedgerPlacement(t *testing.T) {
	for _, paths := range [][]string{{}, {"../outside"}, {"/absolute"}, {"."}, {"a", "a"}} {
		require.Error(t, validatePOCPaths(paths))
	}
	workspace := t.TempDir()
	require.Error(t, pocSeparateLedger(workspace, filepath.Join(workspace, "ledger")))
	require.NoError(t, pocSeparateLedger(workspace, filepath.Dir(workspace)))
	parent := t.TempDir()
	require.NoError(t, os.Symlink(workspace, filepath.Join(parent, "alias")))
	require.Error(t, pocSeparateLedger(workspace, filepath.Join(parent, "alias", "ledger")))
}
