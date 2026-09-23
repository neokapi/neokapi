package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validEvalManifest() EvalManifest {
	return EvalManifest{
		Schema: evalSchema,
		Study:  "kapi-agent-eval",
		Hosts: []PairedAgentSpec{
			{Host: "claude", Model: "claude-sonnet-5", Effort: "high"},
			{Host: "codex", Model: "gpt-5.6-terra", Effort: "medium"},
		},
		Tasks:                 []string{"feature-page", "release-note", "troubleshooting-section"},
		SmokeTask:             "troubleshooting-section",
		Agents:                "all",
		AttemptTimeoutSeconds: 900,
		MaxTurns:              40,
		Billing:               "subscription-only",
	}
}

func TestValidateEvalManifest(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*EvalManifest)
		wantErr string
	}{
		{name: "the shipped manifest", mutate: func(*EvalManifest) {}},
		{"an unsupported schema", func(m *EvalManifest) { m.Schema = 99 }, "unsupported evaluation schema"},
		{"a study name that is not path-safe", func(m *EvalManifest) { m.Study = "Agent Eval" }, "path-safe identifier"},
		{"billing that is not the subscription", func(m *EvalManifest) { m.Billing = "api" }, "subscription-only"},
		{"no host at all", func(m *EvalManifest) { m.Hosts = nil }, "at least one agent host"},
		{"a host kapi does not drive", func(m *EvalManifest) { m.Hosts[0].Host = "emacs" }, `unsupported host "emacs"`},
		{"the same host twice", func(m *EvalManifest) { m.Hosts[1].Host = "claude" }, `duplicate host "claude"`},
		{"a host with no effort setting", func(m *EvalManifest) { m.Hosts[0].Effort = "" }, "explicit model and effort"},
		{"no task", func(m *EvalManifest) { m.Tasks = nil }, "at least one task"},
		{"an unknown task", func(m *EvalManifest) { m.Tasks[0] = "plans-page" }, `unknown evaluation task "plans-page"`},
		{"a task twice", func(m *EvalManifest) { m.Tasks[1] = m.Tasks[0] }, "duplicate task"},
		{"a smoke task outside the selection", func(m *EvalManifest) { m.Tasks = m.Tasks[:2] }, "smoke_task must name a selected task"},
		{"no agent wiring named", func(m *EvalManifest) { m.Agents = "" }, "--agents"},
		{"a timeout outside the permitted range", func(m *EvalManifest) { m.AttemptTimeoutSeconds = 0 }, "attempt_timeout_seconds"},
		{"a turn cap that is not positive", func(m *EvalManifest) { m.MaxTurns = 0 }, "max_turns must be positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validEvalManifest()
			manifest.Hosts = append([]PairedAgentSpec(nil), manifest.Hosts...)
			manifest.Tasks = append([]string(nil), manifest.Tasks...)
			test.mutate(&manifest)
			err := validateEvalManifest(manifest)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestReadEvalManifest(t *testing.T) {
	manifest, err := readEvalManifest(filepath.Join("testdata", "eval-study.json"))
	require.NoError(t, err)
	assert.Equal(t, validEvalManifest(), manifest, "the shipped manifest runs every task on both hosts")
}

func TestReadEvalManifestRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	body, err := json.Marshal(map[string]any{"schema": 1, "study": "x", "unknown": true})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, body, 0o600))
	_, err = readEvalManifest(path)
	require.ErrorContains(t, err, "unknown")
}

func TestEvalSchedule(t *testing.T) {
	manifest := validEvalManifest()

	smoke := evalSchedule(manifest, evalPhaseSmoke)
	require.Len(t, smoke, 2, "a smoke batch is one run per host")
	for _, session := range smoke {
		assert.Equal(t, evalMeasureGrow, session.Measure, "smoke runs Measure 2 only")
		assert.Equal(t, manifest.SmokeTask, session.Task)
	}
	assert.Equal(t, "smoke-troubleshooting-section-claude", smoke[0].ID)

	for phase, measure := range map[string]string{evalPhaseApply: evalMeasureApply, evalPhaseGrow: evalMeasureGrow} {
		sessions := evalSchedule(manifest, phase)
		require.Len(t, sessions, len(manifest.Tasks)*len(manifest.Hosts), "three runs per host per measure")
		for _, session := range sessions {
			assert.Equal(t, measure, session.Measure)
			assert.Equal(t, phase, session.Phase)
		}
	}
	assert.Empty(t, evalSchedule(manifest, evalPhaseReport), "the report runs nothing")

	// Every cell is its own: no two sessions of any phase share one.
	seen := map[string]bool{}
	for _, phase := range evalLivePhases {
		for _, session := range evalSchedule(manifest, phase) {
			assert.False(t, seen[session.ID], "cell %s is shared", session.ID)
			seen[session.ID] = true
		}
	}
}

func TestSelectEvalSessions(t *testing.T) {
	schedule := evalSchedule(validEvalManifest(), evalPhaseGrow)

	all, err := selectEvalSessions(schedule, "")
	require.NoError(t, err)
	assert.Len(t, all, len(schedule))

	picked, err := selectEvalSessions(schedule, schedule[2].ID+","+schedule[0].ID)
	require.NoError(t, err)
	require.Len(t, picked, 2)
	assert.Equal(t, schedule[0].ID, picked[0].ID, "selection keeps the schedule's order")

	_, err = selectEvalSessions(schedule, "grow-release-note-emacs")
	require.ErrorContains(t, err, "outside this phase's schedule")

	_, err = selectEvalSessions(schedule, schedule[0].ID+","+schedule[0].ID)
	require.ErrorContains(t, err, "duplicate")
}
