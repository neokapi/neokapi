package store

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	platstore "github.com/neokapi/neokapi/platform/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAutomationRunStore(t *testing.T) (*AutomationRunStore, *SQLiteStore) {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return NewAutomationRunStore(s.DB()), s
}

func TestAutomationRunStore_RunLifecycle(t *testing.T) {
	store, ss := newTestAutomationRunStore(t)
	ctx := context.Background()

	createRunTestProject(t, ss)

	run := &AutomationRun{
		ProjectID:   "test-proj",
		TriggerType: "connector.push.completed",
		TriggerID:   "evt-1",
		TriggerData: map[string]string{"push_id": "push-1", "items": "en.json"},
		Status:      RunStatusRunning,
	}
	require.NoError(t, store.CreateRun(ctx, run))
	assert.NotEmpty(t, run.ID)

	// Get run.
	got, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, RunStatusRunning, got.Status)
	assert.Equal(t, "push-1", got.TriggerData["push_id"])

	// List runs.
	runs, err := store.ListRuns(ctx, "test-proj", "", 10, 0)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	// Update status.
	require.NoError(t, store.UpdateRunStatus(ctx, run.ID, RunStatusCompleted, ""))
	got, _ = store.GetRun(ctx, run.ID)
	assert.Equal(t, RunStatusCompleted, got.Status)
	assert.NotNil(t, got.EndedAt)
}

func TestAutomationRunStore_StepLifecycle(t *testing.T) {
	store, ss := newTestAutomationRunStore(t)
	ctx := context.Background()

	createRunTestProject(t, ss)

	run := &AutomationRun{ProjectID: "test-proj", TriggerType: "test", Status: RunStatusRunning}
	require.NoError(t, store.CreateRun(ctx, run))

	// Create two steps.
	step1 := &AutomationStep{RunID: run.ID, RuleName: "auto-translate", ActionType: "auto_translate", Status: StepStatusRunning}
	require.NoError(t, store.CreateStep(ctx, step1))

	step2 := &AutomationStep{RunID: run.ID, RuleName: "create-tasks", ActionType: "create_review_tasks", Status: StepStatusRunning}
	require.NoError(t, store.CreateStep(ctx, step2))

	// Check run step count incremented.
	got, _ := store.GetRun(ctx, run.ID)
	assert.Equal(t, 2, got.StepCount)

	// Register jobs on step 1.
	require.NoError(t, store.RegisterStepJobs(ctx, step1.ID, []string{"job-1", "job-2", "job-3"}))

	steps, err := store.ListSteps(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.Equal(t, 3, steps[0].TotalJobs)
	assert.Equal(t, []string{"job-1", "job-2", "job-3"}, steps[0].JobIDs)

	// Register tasks on step 2.
	require.NoError(t, store.RegisterStepTasks(ctx, step2.ID, []string{"task-1", "task-2"}))

	// Complete step 2.
	require.NoError(t, store.UpdateStepStatus(ctx, step2.ID, StepStatusCompleted, ""))
	require.NoError(t, store.IncrementDoneCount(ctx, run.ID))

	got, _ = store.GetRun(ctx, run.ID)
	assert.Equal(t, 1, got.DoneCount)

	// Update job progress on step 1.
	require.NoError(t, store.UpdateStepJobProgress(ctx, step1.ID, 2))
	steps, _ = store.ListSteps(ctx, run.ID)
	assert.Equal(t, 2, steps[0].DoneJobs)
}

func TestAutomationRunStore_Logs(t *testing.T) {
	store, ss := newTestAutomationRunStore(t)
	ctx := context.Background()

	createRunTestProject(t, ss)

	run := &AutomationRun{ProjectID: "test-proj", TriggerType: "test", Status: RunStatusRunning}
	require.NoError(t, store.CreateRun(ctx, run))
	step := &AutomationStep{RunID: run.ID, ActionType: "auto_translate", Status: StepStatusRunning}
	require.NoError(t, store.CreateStep(ctx, step))

	// Append logs.
	logs := []AutomationLog{
		{StepID: step.ID, RunID: run.ID, Level: "info", Message: "Translating en.json for fr-FR"},
		{StepID: step.ID, RunID: run.ID, Level: "info", Message: "Blocks 1-50 of 418"},
		{StepID: step.ID, RunID: run.ID, Level: "error", Message: "Rate limit exceeded", Data: map[string]string{"retry_after": "5s"}},
	}
	require.NoError(t, store.AppendLogs(ctx, logs))

	// List logs.
	got, err := store.ListLogs(ctx, step.ID, 100)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "info", got[0].Level)
	assert.Equal(t, "Translating en.json for fr-FR", got[0].Message)
	assert.Equal(t, "error", got[2].Level)
	assert.Equal(t, "5s", got[2].Data["retry_after"])
}

// createRunTestProject creates a project via SQLiteStore for FK references.
func createRunTestProject(t *testing.T, ss *SQLiteStore) {
	t.Helper()
	p := &platstore.Project{
		ID:                    "test-proj",
		Name:                  "Test",
		DefaultSourceLanguage: model.LocaleID("en"),
	}
	require.NoError(t, ss.CreateProject(context.Background(), p))
}
