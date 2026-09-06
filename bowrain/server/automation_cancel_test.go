package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cancelRun calls the cancel route's handler for one run.
func cancelRun(t *testing.T, srv *Server, projectID, runID string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", nil), rec)
	c.SetParamNames("id", "runId")
	c.SetParamValues(projectID, runID)
	require.NoError(t, srv.HandleCancelAutomationRun(c))
	return rec
}

// openRun seeds a run with one open run_flow step, the shape a cancel meets.
func openRun(t *testing.T, srv *Server, projectID string) (*bstore.AutomationRun, *bstore.AutomationStep) {
	t.Helper()
	ctx := context.Background()
	run := &bstore.AutomationRun{
		ID: id.New(), ProjectID: projectID,
		TriggerType: string(platev.EventPullCompleted), TriggerID: id.New(),
		Status: bstore.RunStatusRunning,
	}
	require.NoError(t, srv.AutomationRunStore.CreateRun(ctx, run))
	step := &bstore.AutomationStep{
		ID: id.New(), RunID: run.ID, RuleName: "long-flow",
		ActionType: "run_flow", Status: bstore.StepStatusRunning,
	}
	require.NoError(t, srv.AutomationRunStore.CreateStep(ctx, step))
	return run, step
}

// TestCancelAutomationRun_ClosesTheRunAndItsSteps proves the route stops a run
// rather than relabelling it: the run ends cancelled and its open step is
// closed with the reason.
func TestCancelAutomationRun_ClosesTheRunAndItsSteps(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projID := seedRunFlowProject(t, srv, "Cancel run")
	run, step := openRun(t, srv, projID)

	rec := cancelRun(t, srv, projID, run.ID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	ctx := context.Background()
	stored, err := srv.AutomationRunStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.RunStatusCancelled, stored.Status)
	assert.Equal(t, "cancelled by user", stored.Error)

	closed, err := srv.AutomationRunStore.GetStep(ctx, step.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.StepStatusSkipped, closed.Status)
}

// TestCancelAutomationRun_RefusesAFinishedRun proves a run that already
// finished keeps its outcome and answers 409.
func TestCancelAutomationRun_RefusesAFinishedRun(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projID := seedRunFlowProject(t, srv, "Cancel finished")
	run, _ := openRun(t, srv, projID)
	require.NoError(t, srv.AutomationRunStore.UpdateRunStatus(context.Background(), run.ID, bstore.RunStatusCompleted, ""))

	rec := cancelRun(t, srv, projID, run.ID)
	require.Equal(t, http.StatusConflict, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "the run has already finished", body.Error)

	stored, err := srv.AutomationRunStore.GetRun(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.RunStatusCompleted, stored.Status)
}

// TestRunFlowObservesTheRunContext proves the run's cancellation reaches the
// flow: an action dispatched under a cancelled context stops there and closes
// its step with the reason rather than running the flow to the end.
func TestRunFlowObservesTheRunContext(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projID := seedRunFlowProject(t, srv, "Cancelled flow")
	_, step := openRun(t, srv, projID)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := srv.executeAutomationAction(ctx,
		event.AutomationAction{Type: "run_flow", Name: "long-flow", Config: map[string]string{"flow": "pseudo-translate"}},
		platev.Event{
			ID: id.New(), Type: platev.EventPullCompleted, ProjectID: projID,
			Data: map[string]string{"items": "en.json", "stream": "main"}, Timestamp: time.Now().UTC(),
		}, step.ID)
	require.NoError(t, err, "dispatch itself succeeds; the action reports its own outcome")

	require.Eventually(t, func() bool {
		s, err := srv.AutomationRunStore.GetStep(context.Background(), step.ID)
		return err == nil && bstore.StepIsTerminal(s.Status)
	}, 10*time.Second, 50*time.Millisecond, "the step never closed")

	closed, err := srv.AutomationRunStore.GetStep(context.Background(), step.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.StepStatusFailed, closed.Status)
	assert.Contains(t, closed.Error, "context canceled")

	// The flow wrote nothing: it stopped before it read a block.
	assert.Empty(t, projectBlock(t, srv, projID).TargetText("fr-FR"))
}
