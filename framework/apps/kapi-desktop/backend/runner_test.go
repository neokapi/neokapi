package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunFlowNoProject(t *testing.T) {
	app := NewApp()
	err := app.RunFlow("test", []string{"file.json"}, "fr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no project open")
}

func TestRunFlowNotFound(t *testing.T) {
	app := NewApp()
	_, _ = app.NewProject("Test", "en", nil)

	err := app.RunFlow("nonexistent", []string{"file.json"}, "fr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunFlowNoInputs(t *testing.T) {
	app := NewApp()
	_, _ = app.NewProject("Test", "en", nil)
	_ = app.SaveFlow("qa", &flow.StepsSpec{
		Steps: []flow.FlowStep{{Tool: "qa-check"}},
	})

	err := app.RunFlow("qa", nil, "fr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no input files")
}

func TestGetRunStateIdle(t *testing.T) {
	app := NewApp()
	assert.Equal(t, "idle", app.GetRunState())
}

func TestCancelRunNoOp(t *testing.T) {
	app := NewApp()
	// Should not panic when no run is active.
	app.CancelRun()
}

func TestRunnerState(t *testing.T) {
	r := newRunner()
	assert.Equal(t, RunStateIdle, r.state)
	assert.False(t, r.running)
}

func TestRunEventTypes(t *testing.T) {
	// Verify RunEvent can represent all event types.
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
