package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunFlowBadTab(t *testing.T) {
	app := NewApp()
	err := app.RunFlow("bad-tab", "test", []string{"file.json"}, []string{"fr"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunFlowNotFound(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "RunTest")

	err := app.RunFlow(tab.ID, "nonexistent", []string{"file.json"}, []string{"fr"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunFlowNoInputs(t *testing.T) {
	app := NewApp()
	tab := newTestProject(t, app, "RunTest2")
	_ = app.SaveFlow(tab.ID, "qa", &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "qa"}},
	})

	err := app.RunFlow(tab.ID, "qa", nil, []string{"fr"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no input files")
}

func TestGetRunStateIdle(t *testing.T) {
	app := NewApp()
	assert.Equal(t, "idle", app.GetRunState())
}

func TestCancelRunNoOp(t *testing.T) {
	app := NewApp()
	app.CancelRun()
}

func TestRunnerState(t *testing.T) {
	r := newRunner()
	assert.Equal(t, RunStateIdle, r.state)
	assert.False(t, r.running)
}

func TestRunEventTypes(t *testing.T) {
	events := []RunEvent{
		{Type: "state", FlowID: "test", Message: "running"},
		{Type: "progress", FlowID: "test", FileIndex: 0, FileCount: 3, FilePath: "a.json"},
		{Type: "error", FlowID: "test", Message: "something failed"},
		{Type: "complete", FlowID: "test", DurationMs: 1234, FilesProcessed: 5},
	}
	for _, e := range events {
		assert.NotEmpty(t, e.Type)
		assert.NotEmpty(t, e.FlowID)
	}
}

func TestGetRunEventsEmpty(t *testing.T) {
	app := NewApp()
	events := app.GetRunEvents()
	assert.Nil(t, events)
}

func TestGetRunEventsAfterInit(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()
	events := app.GetRunEvents()
	assert.Empty(t, events)
}

func TestEmitRunEventAccumulates(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()

	app.emitRunEvent(RunEvent{Type: "state", FlowID: "translate", Message: "running"})
	app.emitRunEvent(RunEvent{Type: "progress", FlowID: "translate", FileIndex: 0, FileCount: 5})
	app.emitRunEvent(RunEvent{Type: "progress", FlowID: "translate", FileIndex: 1, FileCount: 5})

	events := app.GetRunEvents()
	require.Len(t, events, 3)
	assert.Equal(t, "state", events[0].Type)
	assert.Equal(t, "running", events[0].Message)
	assert.Equal(t, "progress", events[1].Type)
	assert.Equal(t, 0, events[1].FileIndex)
	assert.Equal(t, 1, events[2].FileIndex)
}

func TestRunEventsResetOnNewRun(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()

	// Simulate events from a previous run.
	app.emitRunEvent(RunEvent{Type: "state", FlowID: "old", Message: "running"})
	app.emitRunEvent(RunEvent{Type: "complete", FlowID: "old"})
	require.Len(t, app.GetRunEvents(), 2)

	// Simulate starting a new run — events should be cleared.
	app.runState.mu.Lock()
	app.runState.events = nil
	app.runState.mu.Unlock()

	app.emitRunEvent(RunEvent{Type: "state", FlowID: "new", Message: "running"})
	events := app.GetRunEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "new", events[0].FlowID)
}

func TestGetRunEventsReturnsCopy(t *testing.T) {
	app := NewApp()
	app.runState = newRunner()

	app.emitRunEvent(RunEvent{Type: "state", FlowID: "test", Message: "running"})

	events1 := app.GetRunEvents()
	events2 := app.GetRunEvents()

	// Mutating one copy should not affect the other.
	events1[0].Message = "modified"
	assert.Equal(t, "running", events2[0].Message)
}
