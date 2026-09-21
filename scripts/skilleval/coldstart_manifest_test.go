package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validColdStartManifest() ColdStartManifest {
	return ColdStartManifest{
		Schema: coldStartSchema,
		Study:  "kapi-cold-start",
		Hosts: []PairedAgentSpec{
			{Host: "claude", Model: "claude-sonnet-5", Effort: "high"},
			{Host: "codex", Model: "gpt-5.6-terra", Effort: "medium"},
		},
		Tasks:                 []string{"release-note", "trial-ending-email", "empty-export-section"},
		SmokeTask:             "empty-export-section",
		SessionTwoTask:        "plans-page",
		Agents:                "all",
		AttemptTimeoutSeconds: 900,
		MaxTurns:              40,
		Billing:               "subscription-only",
	}
}

func TestValidateColdStartManifest(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ColdStartManifest)
		wantErr string
	}{
		{name: "the shipped manifest", mutate: func(*ColdStartManifest) {}},
		{
			name:    "an unsupported schema",
			mutate:  func(m *ColdStartManifest) { m.Schema = 99 },
			wantErr: "unsupported cold-start schema",
		},
		{
			name:    "a study name that is not path-safe",
			mutate:  func(m *ColdStartManifest) { m.Study = "Cold Start" },
			wantErr: "path-safe identifier",
		},
		{
			name:    "billing that is not the subscription",
			mutate:  func(m *ColdStartManifest) { m.Billing = "api" },
			wantErr: "subscription-only",
		},
		{
			name:    "no host at all",
			mutate:  func(m *ColdStartManifest) { m.Hosts = nil },
			wantErr: "at least one agent host",
		},
		{
			name:    "a host kapi does not drive",
			mutate:  func(m *ColdStartManifest) { m.Hosts[0].Host = "emacs" },
			wantErr: `unsupported host "emacs"`,
		},
		{
			name:    "the same host twice",
			mutate:  func(m *ColdStartManifest) { m.Hosts[1].Host = "claude" },
			wantErr: `duplicate host "claude"`,
		},
		{
			name:    "a host with no effort setting",
			mutate:  func(m *ColdStartManifest) { m.Hosts[0].Effort = "" },
			wantErr: "explicit model and effort",
		},
		{
			name:    "fewer than three session-one tasks",
			mutate:  func(m *ColdStartManifest) { m.Tasks = m.Tasks[:2] },
			wantErr: "at least three tasks",
		},
		{
			name: "no task carrying a wording correction",
			mutate: func(m *ColdStartManifest) {
				m.Tasks = []string{"release-note", "trial-ending-email", "release-note"}
			},
			wantErr: `duplicate task "release-note"`,
		},
		{
			name: "a session-two task running in session one",
			mutate: func(m *ColdStartManifest) {
				m.Tasks = []string{"release-note", "trial-ending-email", "plans-page"}
			},
			wantErr: "cannot run in session one",
		},
		{
			name:    "a smoke task outside the selection",
			mutate:  func(m *ColdStartManifest) { m.SmokeTask = "release-note-two" },
			wantErr: "smoke_task must name a selected session-one task",
		},
		{
			name:    "a session-one task standing in for session two",
			mutate:  func(m *ColdStartManifest) { m.SessionTwoTask = "release-note" },
			wantErr: "is a stage one task",
		},
		{
			name:    "no agent wiring named",
			mutate:  func(m *ColdStartManifest) { m.Agents = "" },
			wantErr: "--agents",
		},
		{
			name:    "a timeout outside the permitted range",
			mutate:  func(m *ColdStartManifest) { m.AttemptTimeoutSeconds = 0 },
			wantErr: "attempt_timeout_seconds",
		},
		{
			name:    "a turn cap that is not positive",
			mutate:  func(m *ColdStartManifest) { m.MaxTurns = 0 },
			wantErr: "max_turns must be positive",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validColdStartManifest()
			test.mutate(&manifest)
			err := validateColdStartManifest(manifest)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

// The drill's premise needs exactly one prompt carrying a person's wording
// correction: with none, nothing cues context_correct; with two, a session that
// recorded one correction cannot be told from a session that recorded both.
func TestValidateColdStartManifestRequiresOneCorrectionTask(t *testing.T) {
	manifest := validColdStartManifest()
	manifest.Tasks = []string{"release-note", "trial-ending-email", "plans-page"}
	manifest.SmokeTask = "release-note"
	manifest.SessionTwoTask = "plans-page"
	require.ErrorContains(t, validateColdStartManifest(manifest), "cannot run in session one")

	stageOne := []string{}
	corrections := 0
	for _, task := range coldStartTasks() {
		if task.Stage != coldStartStageOne {
			continue
		}
		stageOne = append(stageOne, task.ID)
		if task.Correction {
			corrections++
		}
	}
	assert.Equal(t, 1, corrections, "the shipped task set carries one wording correction")
	assert.GreaterOrEqual(t, len(stageOne), 3)
}

func TestReadColdStartManifest(t *testing.T) {
	manifest, err := readColdStartManifest(filepath.Join("testdata", "coldstart-study.json"))
	require.NoError(t, err)
	assert.Equal(t, "kapi-cold-start", manifest.Study)
	assert.Len(t, manifest.Hosts, 2)
	assert.Equal(t, "plans-page", manifest.SessionTwoTask)
}

func TestReadColdStartManifestRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	body, err := json.Marshal(map[string]any{"schema": 1, "study": "x", "unknown": true})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, body, 0o600))
	_, err = readColdStartManifest(path)
	require.ErrorContains(t, err, "unknown")
}

func TestColdStartSchedule(t *testing.T) {
	manifest := validColdStartManifest()

	smoke := coldStartSchedule(manifest, coldStartPhaseSmoke)
	require.Len(t, smoke, 2, "a smoke batch is one first session per host")
	for _, session := range smoke {
		assert.Equal(t, coldStartStageOne, session.Stage)
		assert.Equal(t, manifest.SmokeTask, session.Task)
	}
	assert.Equal(t, "empty-export-section-claude-one", smoke[0].ID)

	one := coldStartSchedule(manifest, coldStartPhaseSessionOne)
	assert.Len(t, one, len(manifest.Tasks)*len(manifest.Hosts))

	two := coldStartSchedule(manifest, coldStartPhaseSessionTwo)
	require.Len(t, two, len(manifest.Tasks)*len(manifest.Hosts))
	for index, session := range two {
		assert.Equal(t, coldStartStageTwo, session.Stage)
		assert.Equal(t, manifest.SessionTwoTask, session.Task,
			"every second session runs the same fresh task")
		assert.Equal(t, one[index].Cell, session.Cell,
			"a second session runs in the cell its first session left behind")
	}
}

func TestSelectColdStartSessions(t *testing.T) {
	schedule := coldStartSchedule(validColdStartManifest(), coldStartPhaseSessionOne)

	all, err := selectColdStartSessions(schedule, "")
	require.NoError(t, err)
	assert.Len(t, all, len(schedule))

	picked, err := selectColdStartSessions(schedule, schedule[2].ID+","+schedule[0].ID)
	require.NoError(t, err)
	require.Len(t, picked, 2)
	assert.Equal(t, schedule[0].ID, picked[0].ID, "selection keeps the schedule's order")

	_, err = selectColdStartSessions(schedule, "release-note-emacs-one")
	require.ErrorContains(t, err, "outside this phase's schedule")

	_, err = selectColdStartSessions(schedule, schedule[0].ID+","+schedule[0].ID)
	require.ErrorContains(t, err, "duplicate")
}
