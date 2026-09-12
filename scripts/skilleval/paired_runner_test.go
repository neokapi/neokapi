package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPairedManifestAndSchedule(t *testing.T) {
	m, err := readPairedManifest("testdata/paired-study.json")
	require.NoError(t, err)
	pilot := pairedSchedule(m, "pilot")
	require.Len(t, pilot, 72)
	require.Len(t, pairedSchedule(m, "smoke"), 6)
	assert.Equal(t, pilot, pairedSchedule(m, "pilot"))
	seen := map[string]bool{}
	for i, s := range pilot {
		require.False(t, seen[s.ID])
		seen[s.ID] = true
		if i%3 != 0 {
			assert.Equal(t, pilot[i-i%3].Task, s.Task)
			assert.Equal(t, pilot[i-i%3].Agent, s.Agent)
			assert.Equal(t, pilot[i-i%3].Repetition, s.Repetition)
		}
	}
	m.Seed++
	assert.NotEqual(t, pilot, pairedSchedule(m, "pilot"))
}

func TestPairedManifestRejectsInvalidInputs(t *testing.T) {
	original, err := readPairedManifest("testdata/paired-study.json")
	require.NoError(t, err)
	cases := map[string]func(*PairedManifest){
		"api billing":    func(m *PairedManifest) { m.Billing = "api" },
		"implicit model": func(m *PairedManifest) { m.Agents[0].Model = "" },
		"mixed arm":      func(m *PairedManifest) { m.Conditions[1] = "mcp+skill" },
		"duplicate task": func(m *PairedManifest) { m.Tasks = append(m.Tasks, m.Tasks[0]) },
		"path traversal": func(m *PairedManifest) { m.Study = "../other" },
		"timeout":        func(m *PairedManifest) { m.AttemptTimeoutSeconds = 0 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(original)
			require.NoError(t, err)
			var m PairedManifest
			require.NoError(t, json.Unmarshal(data, &m))
			change(&m)
			require.Error(t, validatePairedManifest(m))
		})
	}
}

func pairedTestOptions(t *testing.T) PairedOptions {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "bin", "kapi"), []byte("test binary fixture"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "scripts", "skilleval"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "cli", "skills", "data", "kapi"), 0o700))
	return PairedOptions{
		ManifestPath: "testdata/paired-study.json", Phase: "smoke", Dir: t.TempDir(),
		RepoRoot: root, Live: true, MaxAttempts: 2,
	}
}

func fakePairedDependencies(calls *int) pairedDependencies {
	return pairedDependencies{
		prepare: func(_ context.Context, l PairedLaunch) (PairedPrepared, error) {
			return PairedPrepared{Launch: l, Version: "test-version", AuthMode: "subscription", Blockers: []string{}}, nil
		},
		run: func(_ context.Context, p PairedPrepared) (PairedAgentResult, error) {
			*calls++
			return PairedAgentResult{
				Status: "completed", RequestedModel: p.Launch.Agent.Model, ActualModel: p.Launch.Agent.Model,
				Tools: []string{}, QuotaStatus: "unknown",
			}, nil
		},
	}
}

func TestPairedPersistentCeilingAndResume(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 2, calls)
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 2, calls, "process resume must not reset authorization")
	opts.MaxAttempts = 3
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 3, calls)
	opts.Phase = "pilot"
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 3, calls, "phase changes must not reset authorization")
	paths, err := pairedAttemptPaths(opts.Dir)
	require.NoError(t, err)
	require.Len(t, paths, 3)
}

func TestPairedFailuresConsumeAttemptsAndRemainImmutable(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	deps.run = func(_ context.Context, p PairedPrepared) (PairedAgentResult, error) {
		calls++
		return PairedAgentResult{Status: "failed", ActualModel: p.Launch.Agent.Model}, errors.New("fixture failure")
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	paths, err := pairedAttemptPaths(opts.Dir)
	require.NoError(t, err)
	before, err := os.ReadFile(filepath.Join(filepath.Dir(paths[0]), "result.json"))
	require.NoError(t, err)
	opts.MaxAttempts = 6
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 6, calls, "failed sessions are not retried")
	after, err := os.ReadFile(filepath.Join(filepath.Dir(paths[0]), "result.json"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestPairedPreflightAndBlockersNeverInvokeAgent(t *testing.T) {
	for _, phase := range []string{"preflight", "smoke"} {
		t.Run(phase, func(t *testing.T) {
			opts := pairedTestOptions(t)
			opts.Phase = phase
			calls := 0
			deps := fakePairedDependencies(&calls)
			deps.prepare = func(_ context.Context, l PairedLaunch) (PairedPrepared, error) {
				return PairedPrepared{Launch: l, Blockers: []string{"isolation unverified"}, MCPReadiness: &PairedMCPReadiness{Status: "failed", Error: "check_text unavailable", Tools: []string{"check_file"}}}, nil
			}
			err := executePairedWith(context.Background(), opts, deps)
			if phase == "smoke" {
				require.ErrorContains(t, err, "blocked before inference")
				reports, reportErr := filepath.Glob(filepath.Join(opts.Dir, "smoke", "*", "preparation.json"))
				require.NoError(t, reportErr)
				require.NotEmpty(t, reports)
				evidence, readErr := os.ReadFile(reports[0])
				require.NoError(t, readErr)
				assert.Contains(t, string(evidence), "check_text unavailable")
				assert.Contains(t, string(evidence), "check_file")
			} else {
				require.NoError(t, err)
			}
			assert.Zero(t, calls)
			used, err := pairedAttemptsUsed(opts.Dir)
			require.NoError(t, err)
			assert.Zero(t, used)
		})
	}
	opts := pairedTestOptions(t)
	opts.Live = false
	calls := 0
	require.NoError(t, executePairedWith(context.Background(), opts, fakePairedDependencies(&calls)))
	assert.Zero(t, calls)
}

func TestPairedResumeRejectsChangedInputs(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	require.NoError(t, os.WriteFile(filepath.Join(opts.RepoRoot, "scripts", "skilleval", "changed.go"), []byte("changed"), 0o600))
	require.ErrorContains(t, executePairedWith(context.Background(), opts, deps), "study inputs changed")
	assert.Equal(t, 2, calls)
}

func TestPairedScoringRetainsInterruptedAndTamperedArtifacts(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	paths, err := pairedAttemptPaths(opts.Dir)
	require.NoError(t, err)
	var study pairedStudyRecord
	require.NoError(t, readPairedJSON(filepath.Join(opts.Dir, "study.json"), &study))
	dir := filepath.Dir(paths[0])
	require.NoError(t, os.WriteFile(filepath.Join(dir, "workspace", "tampered.txt"), []byte("external edit"), 0o600))
	row, err := scorePairedAttempt(paths[0], study.Fingerprint)
	require.NoError(t, err)
	assert.Equal(t, "changed", row.ArtifactIntegrity)
	assert.False(t, row.ObjectivePassed)
	assert.Nil(t, row.ReviewerMinutes)
	assert.Nil(t, row.DollarCost)
	require.NoError(t, os.Remove(filepath.Join(dir, "result.json")))
	row, err = scorePairedAttempt(paths[0], study.Fingerprint)
	require.NoError(t, err)
	assert.Equal(t, "interrupted", row.Status)
}

func TestPairedAttemptTimeoutAndModelIdentity(t *testing.T) {
	opts := pairedTestOptions(t)
	manifest, err := readPairedManifest(opts.ManifestPath)
	require.NoError(t, err)
	session := pairedSchedule(manifest, "smoke")[0]
	launch, err := materializePairedLaunch(opts, manifest, session, t.TempDir())
	require.NoError(t, err)
	launch.Timeout = time.Millisecond
	deps := pairedDependencies{run: func(ctx context.Context, _ PairedPrepared) (PairedAgentResult, error) {
		<-ctx.Done()
		return PairedAgentResult{ActualModel: session.Agent.Model}, ctx.Err()
	}}
	result := executePairedAttempt(context.Background(), PairedPrepared{Launch: launch}, session, deps)
	assert.Equal(t, "timeout", result.Agent.Status)
	deps.run = func(context.Context, PairedPrepared) (PairedAgentResult, error) {
		return PairedAgentResult{Status: "completed", ActualModel: "wrong-model"}, nil
	}
	result = executePairedAttempt(context.Background(), PairedPrepared{Launch: launch}, session, deps)
	assert.Equal(t, "invalid-identity", result.Agent.Status)
	assert.Contains(t, result.Error, "wrong-model")
}

func TestPairedRateLimitPausesBatch(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	deps.run = func(_ context.Context, p PairedPrepared) (PairedAgentResult, error) {
		calls++
		return PairedAgentResult{Status: "rate-limited", ActualModel: p.Launch.Agent.Model, RateLimited: true}, nil
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 1, calls)
}

func TestPairedCancellationPreservesInterruptedStatus(t *testing.T) {
	opts := pairedTestOptions(t)
	manifest, err := readPairedManifest(opts.ManifestPath)
	require.NoError(t, err)
	session := pairedSchedule(manifest, "smoke")[0]
	launch, err := materializePairedLaunch(opts, manifest, session, t.TempDir())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deps := pairedDependencies{run: func(ctx context.Context, _ PairedPrepared) (PairedAgentResult, error) {
		return PairedAgentResult{}, ctx.Err()
	}}
	result := executePairedAttempt(ctx, PairedPrepared{Launch: launch}, session, deps)
	assert.Equal(t, "interrupted", result.Agent.Status)
	assert.Equal(t, "unavailable", result.IdentityStatus)
	assert.Contains(t, result.Error, "canceled")
}

func TestPairedHashRejectsSpecialAndOversizedFiles(t *testing.T) {
	dir := t.TempDir()
	oversized := filepath.Join(dir, "oversized")
	file, err := os.Create(oversized)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(pairedMaxHashFileBytes+1))
	require.NoError(t, file.Close())
	_, err = pairedTreeHash(dir)
	require.ErrorContains(t, err, "size limit")
	require.NoError(t, os.Remove(oversized))
	target := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink("missing", target))
	_, err = pairedTreeHash(dir)
	require.ErrorContains(t, err, "non-regular")
	require.NoError(t, os.Remove(target))
	fifoBin, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo unavailable")
	}
	require.NoError(t, exec.Command(fifoBin, filepath.Join(dir, "pipe")).Run())
	_, err = pairedTreeHash(dir)
	require.ErrorContains(t, err, "non-regular")
}

func TestPairedVersionChangeBlocksResume(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	opts.MaxAttempts = 6
	deps.prepare = func(_ context.Context, l PairedLaunch) (PairedPrepared, error) {
		return PairedPrepared{Launch: l, Version: "changed", Blockers: []string{}}, nil
	}
	require.ErrorContains(t, executePairedWith(context.Background(), opts, deps), "agent version changed")
	assert.Equal(t, 2, calls)
}

func TestPairedLockPreventsConcurrentRun(t *testing.T) {
	opts := pairedTestOptions(t)
	require.NoError(t, os.WriteFile(filepath.Join(opts.Dir, "runner.lock"), []byte("other run"), 0o600))
	calls := 0
	require.ErrorContains(t, executePairedWith(context.Background(), opts, fakePairedDependencies(&calls)), "acquire study lock")
	assert.Zero(t, calls)
}
