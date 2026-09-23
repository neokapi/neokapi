package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The offline half of a live run, with this checkout's kapi and no model: a
// cell is prepared, a stand-in agent writes a broken first version and a clean
// final one, and the attempt is read back through the shipped surfaces. It is
// what proves that rules load, that the first version is checked as it was
// saved, and that the cell is left at the agent's final state.
func evalTestOptions(t *testing.T) EvalOptions {
	t.Helper()
	root, err := repoRoot()
	require.NoError(t, err)
	kapiBin := findKapi(root)
	if kapiBin == "" {
		t.Skip("no kapi binary: `make build` first")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	return EvalOptions{Manifest: validEvalManifest(), Fixture: evalTestFixture(t), SandboxRoot: t.TempDir(),
		RepoRoot: root, KapiBin: kapiBin}
}

func evalStandIn(write func(ctx context.Context)) evalDependencies {
	return evalDependencies{run: func(ctx context.Context, _ EvalPrepared) (EvalTranscript, error) {
		write(ctx)
		return EvalTranscript{Status: "completed", Calls: []EvalCall{}}, nil
	}}
}

func TestEvalApplyCellChecksBothVersions(t *testing.T) {
	opts := evalTestOptions(t)
	session := evalSessionOf(t, opts.Manifest, evalPhaseApply, "apply-release-note-claude")
	paths, wiring, err := prepareEvalCell(t.Context(), opts, session)
	require.NoError(t, err)
	rules := 0
	for _, c := range opts.Fixture.Key.planted() {
		rules += len(c.Rules)
	}
	assert.Len(t, wiring.Held, rules, "every rule of every planted convention is held")
	require.NotEmpty(t, wiring.Baseline)

	note := filepath.Join(paths.Repo, "docs", "releases", "2026-09.md")
	deps := evalStandIn(func(context.Context) {
		require.NoError(t, os.WriteFile(note, []byte("# September 2026\n\nYou can now log in with your e-mail!\n"), 0o600))
		time.Sleep(3 * evalWatchInterval)
		require.NoError(t, os.WriteFile(note, []byte("# September 2026\n\nYou can now sign in with your email.\n"), 0o600))
	})
	ready := EvalPrepared{Session: session, Paths: paths, Wiring: wiring, Timeout: time.Minute}
	result := executeEvalAttempt(t.Context(), ready, t.TempDir(), deps)
	require.Empty(t, result.Error)
	assert.Equal(t, []string{"docs/releases/2026-09.md"}, result.Changed)
	require.NotNil(t, result.CheckFirst)
	require.NotNil(t, result.CheckFinal)
	assert.ElementsMatch(t, []string{"sign-in", "email", "no-exclamation"}, evalBrokenConventions(opts.Fixture.Key, *result.CheckFirst))
	assert.Empty(t, result.CheckFinal.failing())
	assert.Equal(t, rules, result.HeldAfter)

	data, err := os.ReadFile(note)
	require.NoError(t, err)
	assert.Contains(t, string(data), "sign in with your email", "the cell is left at the final version")
	status := exec.Command("git", "status", "--porcelain")
	status.Dir, status.Env = paths.Repo, evalEnv(paths)
	out, err := status.Output()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(out)), "the session's work is committed and nothing is left over")

	score := scoreEvalApply(opts.Fixture.Key, result.FirstAdded, result.FinalAdded, *result.CheckFirst, *result.CheckFinal, result.Transcript)
	assert.Equal(t, []string{"second-person"}, score.FollowedFirst)
	assert.Equal(t, 0, score.FailingFinal)
}

func TestEvalGrowCellReadsWhatAnAgentRecorded(t *testing.T) {
	opts := evalTestOptions(t)
	session := evalSessionOf(t, opts.Manifest, evalPhaseGrow, "grow-feature-page-codex")
	paths, wiring, err := prepareEvalCell(t.Context(), opts, session)
	require.NoError(t, err)
	assert.Empty(t, wiring.Held, "a Measure 2 cell starts with nothing held")

	deps := evalStandIn(func(ctx context.Context) {
		_, err := evalRunKapiAs(ctx, paths, evalActorAgent, "context", "propose", "Full House",
			"--use", "Fullhouse", "--seen-in", "docs/plans.md")
		require.NoError(t, err)
	})
	ready := EvalPrepared{Session: session, Paths: paths, Wiring: wiring, Timeout: time.Minute}
	result := executeEvalAttempt(t.Context(), ready, t.TempDir(), deps)
	require.Empty(t, result.Error)
	require.Len(t, result.Recorded, 1)
	assert.Equal(t, evalActorAgent, result.Recorded[0].Actor)
	assert.Nil(t, result.CheckFirst, "a Measure 2 run is not checked")

	score := scoreEvalGrow(opts.Fixture, result.Recorded)
	assert.Equal(t, []string{"paid-plan-name"}, score.Recalled)
}
