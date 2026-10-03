package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPairedManifestAndSchedule(t *testing.T) {
	m, err := readPairedManifest("testdata/paired-study.json")
	require.NoError(t, err)
	// The WP5 grid: two hosts, four arms, seven task families, three
	// repetitions.
	assert.Equal(t, pairedConditions, m.Conditions)
	assert.Len(t, m.Tasks, 7)
	assert.Equal(t, 3, m.Repetitions)
	pilot := pairedSchedule(m, "pilot")
	require.Len(t, pilot, 168)
	require.Len(t, pairedSchedule(m, "smoke"), 8)
	assert.Equal(t, pilot, pairedSchedule(m, "pilot"))
	seen := map[string]bool{}
	for i, s := range pilot {
		require.False(t, seen[s.ID])
		seen[s.ID] = true
		if i%4 != 0 {
			assert.Equal(t, pilot[i-i%4].Task, s.Task)
			assert.Equal(t, pilot[i-i%4].Agent, s.Agent)
			assert.Equal(t, pilot[i-i%4].Repetition, s.Repetition)
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
		"no baseline":    func(m *PairedManifest) { m.Conditions = []string{"skill-cli", "mcp"} },
		"only baseline":  func(m *PairedManifest) { m.Conditions = []string{"baseline"} },
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
		// A session that worked for two minutes and failed is an outcome; it
		// does not pause its host.
		return PairedAgentResult{Status: "failed", ActualModel: p.Launch.Agent.Model, DurationMS: 120_000}, errors.New("fixture failure")
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
				// A preflight that finds a blocker fails, so no live phase is
				// started on its say-so.
				require.ErrorContains(t, err, "blockers")
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
	deps.build = func(context.Context, string) (pairedBuild, error) {
		return pairedBuild{Commit: "abc1234", Head: "abc1234def"}, nil
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	require.NoError(t, os.WriteFile(filepath.Join(opts.RepoRoot, "scripts", "skilleval", "changed.go"), []byte("changed"), 0o600))
	err := executePairedWith(context.Background(), opts, deps)
	require.ErrorContains(t, err, "Resume it from "+opts.RepoRoot+" at abc1234def")
	assert.NotContains(t, err.Error(), "choose a fresh")
	assert.Equal(t, 2, calls)
}

// A study records the checkout it runs from and each host's version when it
// starts. A run from another checkout, or with a host upgraded since, is
// refused before a session starts, and says what to do; the same checkout and
// versions resume.
func TestPairedStudyPinsTheCheckoutAndHostVersions(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	versions := map[string]string{"claude": "2.1.287 (Claude Code)", "codex": "codex-cli 0.160.0"}
	deps.hostVersion = func(_ context.Context, host string) (string, error) { return versions[host], nil }
	executables := map[string]string{"claude": "/hosts/claude/claude", "codex": "/hosts/codex/bin/codex"}
	deps.hostExecutable = func(host string) (string, error) { return executables[host], nil }
	deps.prepare = func(_ context.Context, l PairedLaunch) (PairedPrepared, error) {
		return PairedPrepared{Launch: l, Version: versions[l.Agent.Host], AuthMode: "subscription", Blockers: []string{}}, nil
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	require.Equal(t, 2, calls)
	var study pairedStudyRecord
	require.NoError(t, readPairedJSON(filepath.Join(opts.Dir, "study.json"), &study))
	assert.Equal(t, opts.RepoRoot, study.Checkout)
	assert.Equal(t, versions, study.HostVersions)
	assert.Equal(t, executables, study.HostExecutables)

	// A host upgraded since the study started: the refusal names where the
	// study's version ran from and how to put it back.
	opts.MaxAttempts = 4
	versions["codex"] = "codex-cli 0.161.0"
	executables["codex"] = "/opt/homebrew/Caskroom/codex/0.161.0/bin/codex"
	err := executePairedWith(context.Background(), opts, deps)
	require.ErrorContains(t, err, `codex reports "codex-cli 0.161.0", and this study started with "codex-cli 0.160.0", `+
		`run from /hosts/codex/bin/codex`)
	assert.Contains(t, err.Error(), `Put the copy of codex "codex-cli 0.160.0" taken before the study first on PATH`)
	assert.Equal(t, 2, calls)
	versions["codex"] = "codex-cli 0.160.0"

	// The study's own copy back on PATH resumes it, wherever it lies: only the
	// version is pinned.
	executables["codex"] = "/elsewhere/codex/bin/codex"

	// Another checkout.
	other := opts
	other.RepoRoot = pairedTestOptions(t).RepoRoot
	err = executePairedWith(context.Background(), other, deps)
	require.ErrorContains(t, err, "this study runs from the checkout "+opts.RepoRoot)
	assert.Equal(t, 2, calls)

	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 4, calls, "the same checkout and versions resume")
}

// The preflight says where each host runs from, and warns of a host an
// upgrade would replace mid-study: a Homebrew cask or formula directory.
func TestPairedHostNotesWarnOfAnUpgradedInPlaceHost(t *testing.T) {
	notes := pairedHostNotes(map[string]string{
		"claude": "/opt/homebrew/Caskroom/claude-code@latest/2.1.288/claude",
		"codex":  "/study/kapi-wp5-hosts/codex/bin/codex",
	})
	require.Len(t, notes, 2)
	assert.Contains(t, notes[0], "claude runs from /opt/homebrew/Caskroom/claude-code@latest/2.1.288/claude; an upgrade replaces it")
	assert.Equal(t, "codex runs from /study/kapi-wp5-hosts/codex/bin/codex", notes[1])
	assert.True(t, pairedUpgradedInPlace("/usr/local/Cellar/codex/0.160.0/bin/codex"))
	assert.False(t, pairedUpgradedInPlace("/opt/homebrew/bin/claude"), "a link is resolved before it is judged")
}

// A host whose version moves after the study started is refused at its next
// session, its first included: the pin is the study's, not the first attempt's.
func TestPairedSessionOnAnotherHostVersionIsRefused(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	deps.hostVersion = func(_ context.Context, host string) (string, error) { return host + " 1.0", nil }
	deps.prepare = func(_ context.Context, l PairedLaunch) (PairedPrepared, error) {
		return PairedPrepared{Launch: l, Version: l.Agent.Host + " 1.1", AuthMode: "subscription", Blockers: []string{}}, nil
	}
	err := executePairedWith(context.Background(), opts, deps)
	require.ErrorContains(t, err, "agent version changed since the study started")
	assert.Zero(t, calls)
}

// A study runs its own copy of kapi and the skill, taken when it started. A
// rebuild of bin/kapi (whose bytes carry the build date) or an edit to the
// checkout's skill afterwards changes nothing the study measures, and the
// documented resume (make build, then the pilot again) carries on.
func TestPairedResumeSurvivesARebuild(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	deps := fakePairedDependencies(&calls)
	var installed []string
	deps.prepare = func(_ context.Context, l PairedLaunch) (PairedPrepared, error) {
		installed = append(installed, l.KapiBin, l.SkillSource)
		return PairedPrepared{Launch: l, Version: "test-version", AuthMode: "subscription", Blockers: []string{}}, nil
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	require.Equal(t, 2, calls)
	inputs := filepath.Join(opts.Dir, "inputs")
	assert.Equal(t, filepath.Join(inputs, "kapi"), installed[0])
	assert.Equal(t, filepath.Join(inputs, "skills", "kapi"), installed[1])
	copied, err := os.ReadFile(filepath.Join(inputs, "kapi"))
	require.NoError(t, err)
	assert.Equal(t, "test binary fixture", string(copied))

	require.NoError(t, os.WriteFile(filepath.Join(opts.RepoRoot, "bin", "kapi"), []byte("rebuilt at another date"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(opts.RepoRoot, "cli", "skills", "data", "kapi", "SKILL.md"), []byte("edited"), 0o600))
	opts.MaxAttempts = 4
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 4, calls)

	// The study's own copy is checked before every session.
	require.NoError(t, os.Chmod(filepath.Join(inputs, "kapi"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(inputs, "kapi"), []byte("swapped"), 0o700))
	opts.MaxAttempts = 6
	require.ErrorContains(t, executePairedWith(context.Background(), opts, deps), "differ from the ones it started with")
	assert.Equal(t, 4, calls)
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
	var result pairedAttemptResult
	require.NoError(t, readPairedJSON(filepath.Join(dir, "result.json"), &result))
	require.NotNil(t, result.Validation)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "workspace", "tampered.txt"), []byte("external edit"), 0o600))
	row, err := scorePairedAttempt(paths[0], study.Fingerprint)
	require.NoError(t, err)
	assert.Equal(t, "changed", row.ArtifactIntegrity)
	// The verdict is the one recorded when the attempt finished.
	assert.Equal(t, result.Validation.ObjectivePassed, row.ObjectivePassed)
	assert.Nil(t, row.ReviewerMinutes)
	assert.Nil(t, row.DollarCost)
	require.NoError(t, os.Remove(filepath.Join(dir, "result.json")))
	row, err = scorePairedAttempt(paths[0], study.Fingerprint)
	require.NoError(t, err)
	assert.Equal(t, "interrupted", row.Status)
}

// A reviewer's git status, or a kapi command run in a workspace, rewrites
// git's index and kapi's runtime folders. Neither touches what was graded, so
// the attempt's integrity stays verified.
func TestPairedReviewInTheWorkspaceKeepsIntegrity(t *testing.T) {
	opts := pairedTestOptions(t)
	calls := 0
	require.NoError(t, executePairedWith(context.Background(), opts, fakePairedDependencies(&calls)))
	paths, err := pairedAttemptPaths(opts.Dir)
	require.NoError(t, err)
	var study pairedStudyRecord
	require.NoError(t, readPairedJSON(filepath.Join(opts.Dir, "study.json"), &study))
	workspace := filepath.Join(filepath.Dir(paths[0]), "workspace")
	for _, dir := range []string{".git", "kapi-data", ".kapi/work"} {
		require.NoError(t, os.MkdirAll(filepath.Join(workspace, dir), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(workspace, dir, "index"), []byte("refreshed"), 0o600))
	}
	row, err := scorePairedAttempt(paths[0], study.Fingerprint)
	require.NoError(t, err)
	assert.Equal(t, "verified", row.ArtifactIntegrity)
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

// A rate limit pauses the host whose subscription hit it; the other host's
// sessions go on.
func TestPairedRateLimitPausesItsHost(t *testing.T) {
	opts := pairedTestOptions(t)
	opts.MaxAttempts = 8
	limited := ""
	hosts := []string{}
	deps := fakePairedDependencies(new(int))
	deps.run = func(_ context.Context, p PairedPrepared) (PairedAgentResult, error) {
		host := p.Launch.Agent.Host
		hosts = append(hosts, host)
		if limited == "" {
			limited = host
		}
		if host != limited {
			return PairedAgentResult{Status: "completed", ActualModel: p.Launch.Agent.Model}, nil
		}
		return PairedAgentResult{Status: "rate_limited", ActualModel: p.Launch.Agent.Model, RateLimited: true}, nil
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	count := 0
	for _, host := range hosts {
		if host == limited {
			count++
		}
	}
	assert.Equal(t, 1, count, "the limited host starts nothing after its rate limit")
	assert.Len(t, hosts, 5, "the other host runs its four smoke sessions")
}

// -paired-retry runs a rate-limited attempt again and keeps the first record,
// scored as superseded and counted against the ceiling.
func TestPairedRetryRunsRateLimitedAttemptsAgain(t *testing.T) {
	opts := pairedTestOptions(t)
	opts.MaxAttempts = 1
	calls := 0
	deps := fakePairedDependencies(&calls)
	deps.run = func(_ context.Context, p PairedPrepared) (PairedAgentResult, error) {
		calls++
		return PairedAgentResult{Status: "rate_limited", ActualModel: p.Launch.Agent.Model, RateLimited: true}, nil
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	require.Equal(t, 1, calls)
	opts.MaxAttempts = 2
	opts.Retry = true
	deps.run = fakePairedDependencies(&calls).run
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 2, calls)
	paths, err := pairedAttemptPaths(opts.Dir)
	require.NoError(t, err)
	require.Len(t, paths, 2)
	var study pairedStudyRecord
	require.NoError(t, readPairedJSON(filepath.Join(opts.Dir, "study.json"), &study))
	superseded := 0
	for _, path := range paths {
		row, err := scorePairedAttempt(path, study.Fingerprint)
		require.NoError(t, err)
		if row.Superseded {
			superseded++
			assert.Equal(t, "rate_limited", row.Status)
		}
	}
	assert.Equal(t, 1, superseded)
}

// Concurrent sessions never exceed the ceiling, and each host runs one
// session at a time when the concurrency equals the number of hosts.
func TestPairedConcurrencyKeepsTheCeilingAndOneSessionPerHost(t *testing.T) {
	opts := pairedTestOptions(t)
	opts.Concurrency = 2
	opts.MaxAttempts = 7
	opts.Phase = "pilot"
	var mu sync.Mutex
	running := map[string]int{}
	most := map[string]int{}
	calls := 0
	deps := fakePairedDependencies(&calls)
	deps.run = func(_ context.Context, p PairedPrepared) (PairedAgentResult, error) {
		host := p.Launch.Agent.Host
		mu.Lock()
		calls++
		running[host]++
		most[host] = max(most[host], running[host])
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		running[host]--
		mu.Unlock()
		return PairedAgentResult{Status: "completed", ActualModel: p.Launch.Agent.Model}, nil
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 7, calls)
	for host, n := range most {
		assert.Equal(t, 1, n, "host %s ran more than one session at a time", host)
	}
	used, err := pairedAttemptsUsed(opts.Dir)
	require.NoError(t, err)
	assert.Equal(t, 7, used)
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

// Sessions that keep ending within seconds pause their host, whatever the
// status says; an infrastructure failure can be run again.
func TestPairedQuickFailuresPauseTheHostAndInfraFailuresRetry(t *testing.T) {
	opts := pairedTestOptions(t)
	opts.MaxAttempts = 8
	failing := ""
	hosts := []string{}
	deps := fakePairedDependencies(new(int))
	deps.run = func(_ context.Context, p PairedPrepared) (PairedAgentResult, error) {
		host := p.Launch.Agent.Host
		hosts = append(hosts, host)
		if failing == "" {
			failing = host
		}
		if host != failing {
			return PairedAgentResult{Status: "completed", ActualModel: p.Launch.Agent.Model, DurationMS: 90_000}, nil
		}
		return PairedAgentResult{Status: "infra_failed", InfraFailure: "network", ActualModel: p.Launch.Agent.Model, DurationMS: 3_000},
			errors.New("stream disconnected")
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	count := 0
	for _, host := range hosts {
		if host == failing {
			count++
		}
	}
	assert.Equal(t, 2, count, "the failing host stops after two quick failures in a row")
	assert.Len(t, hosts, 6, "the other host runs its four smoke sessions")

	// Once the network is back, the two failed attempts run again.
	opts.Retry = true
	opts.MaxAttempts = 10
	calls := 0
	deps.run = fakePairedDependencies(&calls).run
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Equal(t, 4, calls, "two retried and two never started")
	used, err := pairedAttemptsUsed(opts.Dir)
	require.NoError(t, err)
	assert.Equal(t, 10, used, "superseded attempts still count against the ceiling")
}

// An expired login pauses its host at once: every later session would fail
// the same way.
func TestPairedAuthFailurePausesItsHost(t *testing.T) {
	opts := pairedTestOptions(t)
	opts.MaxAttempts = 8
	failing := ""
	hosts := []string{}
	deps := fakePairedDependencies(new(int))
	deps.run = func(_ context.Context, p PairedPrepared) (PairedAgentResult, error) {
		host := p.Launch.Agent.Host
		hosts = append(hosts, host)
		if failing == "" {
			failing = host
		}
		if host != failing {
			return PairedAgentResult{Status: "completed", ActualModel: p.Launch.Agent.Model}, nil
		}
		return PairedAgentResult{Status: "infra_failed", InfraFailure: "auth", ActualModel: p.Launch.Agent.Model, DurationMS: 120_000},
			errors.New("OAuth token has expired")
	}
	require.NoError(t, executePairedWith(context.Background(), opts, deps))
	assert.Len(t, hosts, 5)
}

// The summary splits the stale task's attempts by when the other editor's
// change landed, lists the change-set fields that failed to decode, and
// leaves out attempts an infrastructure failure cut short.
func TestPairedSummaryShowsStaleRecoveryAndDecodeErrors(t *testing.T) {
	session := func(condition string) PairedSession {
		return PairedSession{Task: "recover-stale-read", Agent: PairedAgentSpec{Host: "claude"}, Condition: condition}
	}
	rows := []pairedScoreRow{
		{Session: session("mcp"), Phase: "pilot", Status: "completed", ObjectivePassed: true,
			Refusals:     map[string]int{"stale": 1},
			Interference: &PairedInterferenceRecord{Triggered: true, Applied: true}},
		{Session: session("mcp"), Phase: "pilot", Status: "completed", ObjectivePassed: true,
			Interference: &PairedInterferenceRecord{Triggered: true, Applied: true, AgentWroteFirst: true}},
		{Session: session("mcp"), Phase: "pilot", Status: "completed", ObjectivePassed: false,
			Refusals:     map[string]int{"invalid:/type": 2},
			Interference: &PairedInterferenceRecord{}, OutsideCell: []string{"temp:/tmp/x.json"}},
		{Session: session("mcp"), Phase: "pilot", Status: "infra_failed"},
	}
	var markdown strings.Builder
	writePairedSummary(&markdown, rows)
	text := markdown.String()
	assert.Contains(t, text, "1 attempts are left out")
	assert.Contains(t, text, "| recover-stale-read | claude | mcp | 3 | 2 | 3 |")
	assert.Contains(t, text, "| 1 | 1 |\n", "outside cell and changed columns")
	assert.Contains(t, text, "## Stale recovery")
	assert.Contains(t, text, "| recover-stale-read | claude | mcp | 3 | 1 | 1 | 1 | 1 | 1 | 0 | 1 | 1 |")
	assert.Contains(t, text, "## Change sets that did not decode")
	assert.Contains(t, text, "| recover-stale-read | claude | mcp | /type 2 |")
}

// A disk that fills during a session fails the agent's tools, so the attempt
// is an infrastructure failure, run again rather than scored, however the
// session ended.
func TestPairedAFullDiskIsAnInfrastructureFailure(t *testing.T) {
	opts := pairedTestOptions(t)
	manifest, err := readPairedManifest(opts.ManifestPath)
	require.NoError(t, err)
	session := pairedSchedule(manifest, "smoke")[0]
	launch, err := materializePairedLaunch(opts, manifest, session, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(launch.TranscriptPath), 0o755))
	require.NoError(t, os.WriteFile(launch.TranscriptPath,
		[]byte(`{"type":"user","message":{"content":[{"type":"tool_result","content":"write out/nb.xliff: no space left on device"}]}}`+"\n"), 0o644))
	deps := pairedDependencies{run: func(context.Context, PairedPrepared) (PairedAgentResult, error) {
		return PairedAgentResult{Status: "completed", ActualModel: session.Agent.Model}, nil
	}}
	result := executePairedAttempt(context.Background(), PairedPrepared{Launch: launch}, session, deps)
	assert.Equal(t, "infra_failed", result.Agent.Status)
	assert.Equal(t, "disk", result.Agent.InfraFailure)
	assert.Contains(t, pairedRetryStatuses, result.Agent.Status, "run again on retry")
}
