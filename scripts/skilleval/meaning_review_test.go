package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func meaningTestOptions(t *testing.T) MeaningOptions {
	t.Helper()
	opts := MeaningOptions{Inputs: t.TempDir(), Dir: t.TempDir(), RepoRoot: t.TempDir(), Live: true, MaxAttempts: 2}
	require.NoError(t, os.MkdirAll(filepath.Join(opts.RepoRoot, "scripts", "skilleval"), 0o700))
	manifest := meaningManifest{Schema: 1, Study: "meaning-test", Billing: "subscription-only", AttemptTimeoutSeconds: 180, MaxTurns: 3,
		Agents: []PairedAgentSpec{{Host: "claude", Model: "test-model", Effort: "high"}}, Sessions: []meaningSession{}}
	lines := []string{}
	for _, id := range []string{"one", "two", "three"} {
		manifest.Sessions = append(manifest.Sessions, meaningSession{ID: id, Host: "claude", CaseID: id})
		input := meaningInput{ID: id, ReaderTask: "Invite a guest", Audience: "workspace owners", Surface: "guide", Destination: "help",
			Sources: []meaningSource{{ID: "policy", Text: "Only owners can invite guests."}}, Candidate: "Members can invite guests.", Variables: map[string]any{}}
		body, err := json.Marshal(input)
		require.NoError(t, err)
		lines = append(lines, string(body))
	}
	require.NoError(t, writePairedJSON(filepath.Join(opts.Inputs, "manifest.json"), manifest))
	require.NoError(t, os.WriteFile(filepath.Join(opts.Inputs, "inputs.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(opts.Inputs, "instruction.txt"), []byte("Review meaning against supplied evidence."), 0o600))
	return opts
}

func fakeMeaningDependencies(t *testing.T, calls *int) pairedDependencies {
	t.Helper()
	deps := fakePairedDependencies(calls)
	deps.run = func(_ context.Context, prepared PairedPrepared) (PairedAgentResult, error) {
		*calls++
		require.True(t, prepared.Launch.NoTools)
		require.Equal(t, "baseline", prepared.Launch.Condition)
		require.FileExists(t, filepath.Join(filepath.Dir(prepared.Launch.Workspace), "started.json"), "started ledger must precede inference")
		require.Contains(t, prepared.Launch.Prompt, "Members can invite guests.")
		require.Contains(t, prepared.Launch.Prompt, "Only owners can invite guests.")
		files, err := os.ReadDir(prepared.Launch.Workspace)
		require.NoError(t, err)
		require.Empty(t, files, "fixed text is supplied in the prompt; workspace has no additional evidence")
		return PairedAgentResult{Status: "completed", RequestedModel: "test-model", ActualModel: "test-model", Tools: []string{}, FinalText: `{"findings":[],"abstentions":[]}`}, nil
	}
	return deps
}

func TestMeaningCapResumeAndInterruptedReservation(t *testing.T) {
	opts := meaningTestOptions(t)
	calls := 0
	deps := fakeMeaningDependencies(t, &calls)
	require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
	require.Equal(t, 2, calls)
	require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
	require.Equal(t, 2, calls, "resume cannot reset the persisted attempt count")
	require.NoError(t, os.Mkdir(filepath.Join(opts.Dir, "attempts", "three"), 0o700))
	opts.MaxAttempts = 3
	require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
	require.Equal(t, 2, calls, "an interrupted reserved attempt cannot be retried")
}

func TestMeaningChangedInputsAndCodeRejected(t *testing.T) {
	for _, change := range []string{"instruction", "code", "corpus", "manifest"} {
		t.Run(change, func(t *testing.T) {
			opts := meaningTestOptions(t)
			calls := 0
			deps := fakeMeaningDependencies(t, &calls)
			require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
			path := filepath.Join(opts.Inputs, "instruction.txt")
			switch change {
			case "code":
				path = filepath.Join(opts.RepoRoot, "scripts", "skilleval", "changed.go")
			case "corpus":
				path = filepath.Join(opts.Inputs, "inputs.jsonl")
			case "manifest":
				path = filepath.Join(opts.Inputs, "manifest.json")
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			require.NoError(t, err)
			_, err = f.WriteString(" ")
			require.NoError(t, err)
			require.NoError(t, f.Close())
			require.Error(t, executeMeaningWith(context.Background(), opts, deps))
			require.Equal(t, 2, calls)
		})
	}
}

func TestMeaningOfflineNeverCallsRun(t *testing.T) {
	opts := meaningTestOptions(t)
	opts.Live = false
	calls := 0
	deps := fakeMeaningDependencies(t, &calls)
	require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
	require.Zero(t, calls)
	require.NoDirExists(t, filepath.Join(opts.Dir, "attempts"))
}

func TestMeaningRateLimitStopsAndPersists(t *testing.T) {
	opts := meaningTestOptions(t)
	calls := 0
	deps := fakeMeaningDependencies(t, &calls)
	deps.run = func(context.Context, PairedPrepared) (PairedAgentResult, error) {
		calls++
		return PairedAgentResult{Status: "rate_limited", RateLimited: true, Tools: []string{}}, errors.New("quota")
	}
	require.ErrorContains(t, executeMeaningWith(context.Background(), opts, deps), "rate limit")
	require.Equal(t, 1, calls)
	require.ErrorContains(t, executeMeaningWith(context.Background(), opts, deps), "rate limit")
	require.Equal(t, 1, calls)
	require.NoError(t, os.Remove(filepath.Join(opts.Dir, "rate-limit-stop.json")))
	require.ErrorContains(t, executeMeaningWith(context.Background(), opts, deps), "rate-limited attempt")
	require.Equal(t, 1, calls, "the retained result also protects a crash before stop-file persistence")
}

func TestMeaningMalformedAndToolResultsCannotPassIntegrity(t *testing.T) {
	for _, status := range []string{"malformed", "tools", "failed"} {
		t.Run(status, func(t *testing.T) {
			opts := meaningTestOptions(t)
			opts.MaxAttempts = 1
			calls := 0
			deps := fakeMeaningDependencies(t, &calls)
			deps.run = func(context.Context, PairedPrepared) (PairedAgentResult, error) {
				result := PairedAgentResult{Status: "completed", Tools: []string{}, FinalText: `{"findings":[],"abstentions":[]}`}
				switch status {
				case "malformed":
					result.FinalText = "I found no problems."
				case "tools":
					result.Tools = []string{"Read"}
				case "failed":
					result.Status = "identity_unverified"
				}
				return result, nil
			}
			require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
			var result meaningResult
			require.NoError(t, readPairedJSON(filepath.Join(opts.Dir, "attempts", "one", "result.json"), &result))
			assert.False(t, result.Integrity.Valid)
			assert.NotEmpty(t, result.Integrity.Errors)
		})
	}
}

func TestMeaningEvidenceIntegrityIsNotSemanticScoring(t *testing.T) {
	input := meaningInput{Candidate: "Members can invite guests.", Sources: []meaningSource{{ID: "policy", Text: "Only owners can invite guests."}}}
	cases := []struct {
		name, output string
		valid        bool
	}{
		{name: "empty findings", output: `{"findings":[],"abstentions":[]}`, valid: true},
		{name: "supported shape", output: `{"findings":[{"candidate_quote":"Members","source_ids":["policy"],"kind":"conflict","rationale":"Owners are required."}],"abstentions":[]}`, valid: true},
		{name: "nonsense rationale still structural", output: `{"findings":[{"candidate_quote":"Members","source_ids":["policy"],"kind":"conflict","rationale":"The moon is green."}],"abstentions":[]}`, valid: true},
		{name: "omission empty quote", output: `{"findings":[{"candidate_quote":"","source_ids":["policy"],"kind":"omission","rationale":"Missing restriction."}],"abstentions":[]}`, valid: true},
		{name: "missing evidence abstention", output: `{"findings":[],"abstentions":[{"candidate_quote":"","source_ids":[],"missing_evidence":"Invitation limits are absent."}]}`, valid: true},
		{name: "invented quote", output: `{"findings":[{"candidate_quote":"Everyone","source_ids":["policy"],"kind":"conflict","rationale":"Wrong role."}],"abstentions":[]}`},
		{name: "invented source", output: `{"findings":[{"candidate_quote":"Members","source_ids":["unknown"],"kind":"conflict","rationale":"Wrong role."}],"abstentions":[]}`},
		{name: "missing fields", output: `{}`},
		{name: "extra field", output: `{"findings":[],"abstentions":[],"pass":true}`},
		{name: "trailing output", output: `{"findings":[],"abstentions":[]} {}`},
		{name: "conflict empty quote", output: `{"findings":[{"candidate_quote":"","source_ids":["policy"],"kind":"conflict","rationale":"Wrong role."}],"abstentions":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validateMeaningReview(tc.output, input)
			assert.Equal(t, tc.valid, result.Valid, result.Errors)
			assert.Contains(t, result.Scope, "semantic correctness unmeasured")
		})
	}
}

func TestMeaningStreamRejectsToolUse(t *testing.T) {
	cases := []struct{ host, event string }{
		{host: "claude", event: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{}}]}}`},
		{host: "codex", event: `{"type":"item.started","item":{"type":"command_execution","command":"pwd"}}`},
		{host: "codex", event: `{"type":"item.completed","item":{"type":"file_change"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.host+tc.event, func(t *testing.T) {
			result, err := parsePairedAgentStream(strings.NewReader(tc.event+"\n"), PairedLaunch{Agent: PairedAgentSpec{Host: tc.host, Model: "test-model"}, Condition: "baseline", NoTools: true})
			require.ErrorContains(t, err, "tool use violates")
			assert.Equal(t, "tool_use_violation", result.Status)
			assert.NotEmpty(t, result.Tools)
		})
	}
}

func TestMeaningClaudeDisablesToolsWithoutLeakingCredentials(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "private-test-token")
	prepared := PairedPrepared{Args: []string{}, Env: []string{}, Blockers: []string{}, IsolationNotes: []string{},
		Launch: PairedLaunch{Agent: PairedAgentSpec{Host: "claude", Model: "test-model", Effort: "high"}, Condition: "baseline",
			StateDir: t.TempDir(), Workspace: t.TempDir(), MaxTurns: 3, NoTools: true}}
	require.NoError(t, preparePairedClaude(context.Background(), &prepared))
	require.Empty(t, prepared.Blockers)
	found := false
	for i, arg := range prepared.Args {
		if arg == "--tools" {
			found = true
			assert.Empty(t, prepared.Args[i+1])
		}
	}
	require.True(t, found)
	body, err := json.Marshal(prepared)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "private-test-token")
}
