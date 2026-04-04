package event

import (
	"context"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRunStore(t *testing.T) *bstore.AutomationRunStore {
	t.Helper()
	ss, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { ss.Close() })

	// Create a project for FK.
	p := &platstore.Project{ID: "proj-1", Name: "Test", DefaultSourceLanguage: model.LocaleID("en")}
	require.NoError(t, ss.CreateProject(context.Background(), p))

	return bstore.NewAutomationRunStore(ss.DB())
}

func TestRunManager_CreatesRunAndStep(t *testing.T) {
	store := newTestRunStore(t)
	ctx := context.Background()

	var executed []string
	var mu sync.Mutex
	executor := func(action AutomationAction, ev platev.Event, stepID string) error {
		mu.Lock()
		executed = append(executed, action.Type+":"+stepID)
		mu.Unlock()
		return nil
	}

	rm := NewAutomationRunManager(store, executor)

	ev := platev.Event{
		ID:        "evt-1",
		Type:      platev.EventPushAutomationsCompleted,
		ProjectID: "proj-1",
		Data:      map[string]string{"push_id": "p1"},
	}

	action := AutomationAction{
		Type: "create_review_tasks",
		Name: "test-rule",
	}

	err := rm.Execute(action, ev)
	require.NoError(t, err)

	// Verify run was created.
	runs, err := store.ListRuns(ctx, "proj-1", "", 10, 0)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "proj-1", runs[0].ProjectID)
	assert.Equal(t, string(platev.EventPushAutomationsCompleted), runs[0].TriggerType)

	// Verify step was created.
	steps, err := store.ListSteps(ctx, runs[0].ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, "create_review_tasks", steps[0].ActionType)
	assert.Equal(t, "test-rule", steps[0].RuleName)
	// Synchronous action → step should be completed.
	assert.Equal(t, bstore.StepStatusCompleted, steps[0].Status)

	// Verify executor was called with a step ID.
	mu.Lock()
	require.Len(t, executed, 1)
	assert.Contains(t, executed[0], "create_review_tasks:")
	assert.NotEqual(t, "create_review_tasks:", executed[0]) // stepID is non-empty
	mu.Unlock()
}

func TestRunManager_GroupsSameEvent(t *testing.T) {
	store := newTestRunStore(t)
	ctx := context.Background()

	executor := func(action AutomationAction, ev platev.Event, stepID string) error {
		return nil
	}

	rm := NewAutomationRunManager(store, executor)

	ev := platev.Event{
		ID:        "evt-2",
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
	}

	// Two actions from the same event.
	_ = rm.Execute(AutomationAction{Type: "auto_translate", Name: "rule-1"}, ev)
	_ = rm.Execute(AutomationAction{Type: "auto_extract", Name: "rule-2"}, ev)

	// Should create 1 run with 2 steps.
	runs, err := store.ListRuns(ctx, "proj-1", "", 10, 0)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, 2, runs[0].StepCount)

	steps, err := store.ListSteps(ctx, runs[0].ID)
	require.NoError(t, err)
	assert.Len(t, steps, 2)
}

func TestRunManager_NilStorePassesThrough(t *testing.T) {
	var called bool
	executor := func(action AutomationAction, ev platev.Event, stepID string) error {
		called = true
		assert.Empty(t, stepID) // no step tracking without store
		return nil
	}

	rm := NewAutomationRunManager(nil, executor)
	err := rm.Execute(AutomationAction{Type: "notify"}, platev.Event{ID: "evt-3"})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestRunManager_LogsOnSteps(t *testing.T) {
	store := newTestRunStore(t)
	ctx := context.Background()

	executor := func(action AutomationAction, ev platev.Event, stepID string) error {
		return nil
	}

	rm := NewAutomationRunManager(store, executor)

	_ = rm.Execute(
		AutomationAction{Type: "notify", Name: "test"},
		platev.Event{ID: "evt-4", ProjectID: "proj-1"},
	)

	runs, _ := store.ListRuns(ctx, "proj-1", "", 10, 0)
	require.Len(t, runs, 1)
	steps, _ := store.ListSteps(ctx, runs[0].ID)
	require.Len(t, steps, 1)

	// Should have "Starting action" + "Action completed" logs.
	logs, err := store.ListLogs(ctx, steps[0].ID, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(logs), 2)
	assert.Equal(t, "Starting action: notify", logs[0].Message)
}

func TestRunManager_AsyncStepStaysRunning(t *testing.T) {
	store := newTestRunStore(t)
	ctx := context.Background()

	executor := func(action AutomationAction, ev platev.Event, stepID string) error {
		return nil
	}

	rm := NewAutomationRunManager(store, executor)

	_ = rm.Execute(
		AutomationAction{Type: "auto_translate", Name: "translate-rule"},
		platev.Event{ID: "evt-5", ProjectID: "proj-1"},
	)

	runs, _ := store.ListRuns(ctx, "proj-1", "", 10, 0)
	require.Len(t, runs, 1)
	steps, _ := store.ListSteps(ctx, runs[0].ID)
	require.Len(t, steps, 1)

	// Async action → step stays running (not completed immediately).
	assert.Equal(t, bstore.StepStatusRunning, steps[0].Status)

	// Run should still be running.
	assert.Equal(t, bstore.RunStatusRunning, runs[0].Status)
}

// Small sleep helper for async operations.
func init() {
	_ = time.Millisecond // used by tests
}
