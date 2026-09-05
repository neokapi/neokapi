package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// automationRunFrame is the one shape every frame of the run stream carries.
type automationRunFrame struct {
	Type  string                   `json:"type"`
	Run   *bstore.AutomationRun    `json:"run"`
	Steps []*bstore.AutomationStep `json:"steps"`
}

// runStreamFrames decodes the run frames of an SSE body, in order. The
// closing `event: done` carries no run and is left out.
func runStreamFrames(t *testing.T, body string) []automationRunFrame {
	t.Helper()
	var frames []automationRunFrame
	for chunk := range strings.SplitSeq(body, "\n\n") {
		if strings.HasPrefix(chunk, "event: ") {
			continue
		}
		for line := range strings.SplitSeq(chunk, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var f automationRunFrame
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &f), "frame: %s", line)
			frames = append(frames, f)
		}
	}
	return frames
}

// openRunStream starts the SSE handler for the run in a goroutine and returns
// the recorder, the request cancel, and the channel the handler's return
// lands on.
func openRunStream(t *testing.T, srv *Server, projectID, runID string) (*syncRecorder, context.CancelFunc, <-chan error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+projectID+"/automations/runs/"+runID+"/events", nil)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	rec := newSyncRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "runId")
	c.SetParamValues(projectID, runID)
	done := make(chan error, 1)
	go func() { done <- srv.HandleAutomationRunSSE(c) }()
	return rec, cancel, done
}

// TestHandleAutomationRunSSE_PushesTransitions proves the stream delivers a
// step closing and the run settling as the executor persists them, well
// inside the three-second snapshot interval, and that a closed stream leaves
// no subscriber behind.
func TestHandleAutomationRunSSE_PushesTransitions(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projectID := seedRunFlowProject(t, srv, "sse-push")
	ctx := context.Background()

	// Hold the real executor at the step so the run exists, and can be
	// subscribed to, before its step closes.
	release := make(chan struct{})
	started := make(chan string, 1)
	rm := event.NewAutomationRunManager(srv.AutomationRunStore, func(a event.AutomationAction, ev platev.Event, stepID string) error {
		started <- stepID
		<-release
		return srv.executeAutomationAction(a, ev, stepID)
	})
	rm.SetRunNotifier(srv.runHub)

	executed := make(chan error, 1)
	go func() {
		executed <- rm.Execute(
			event.AutomationAction{Type: "notify", Name: "tell-me", Config: map[string]string{"user_id": "u1"}},
			platev.Event{ID: id.New(), Type: platev.EventPullCompleted, ProjectID: projectID, Timestamp: time.Now().UTC()},
		)
	}()

	var stepID string
	select {
	case stepID = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the executor was never reached")
	}
	step, err := srv.AutomationRunStore.GetStep(ctx, stepID)
	require.NoError(t, err)
	runID := step.RunID

	rec, cancel, handlerDone := openRunStream(t, srv, projectID, runID)
	defer cancel()

	// The stream opens with a snapshot of the running run.
	require.Eventually(t, func() bool {
		return strings.Contains(rec.body(), `"type":"snapshot"`)
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 1, srv.runHub.subscribers(runID))
	opening := runStreamFrames(t, rec.body())
	require.Len(t, opening, 1)
	assert.Equal(t, bstore.RunStatusRunning, opening[0].Run.Status)
	require.Len(t, opening[0].Steps, 1)
	assert.Equal(t, bstore.StepStatusRunning, opening[0].Steps[0].Status)

	// Let the action finish: the step closes and the run settles, and both
	// reach the stream as pushes, before any snapshot tick.
	close(release)
	require.Eventually(t, func() bool {
		return strings.Contains(rec.body(), `"type":"run.finished"`)
	}, 1500*time.Millisecond, 5*time.Millisecond, "the settled run was not pushed inside the snapshot interval")
	require.NoError(t, <-executed)

	frames := runStreamFrames(t, rec.body())
	var kinds []string
	for _, f := range frames {
		kinds = append(kinds, f.Type)
	}
	assert.Equal(t, []string{"snapshot", "step.finished", "run.finished"}, kinds,
		"one opening snapshot, then pushes; the ticker has not fired")

	last := frames[len(frames)-1]
	assert.Equal(t, bstore.RunStatusCompleted, last.Run.Status)
	assert.Equal(t, 1, last.Run.DoneCount)
	require.Len(t, last.Steps, 1)
	assert.Equal(t, bstore.StepStatusCompleted, last.Steps[0].Status)
	assert.NotContains(t, rec.body(), "event: done", "a pushed settle does not close the stream; the snapshot tick does")

	// Disconnecting unsubscribes cleanly.
	cancel()
	select {
	case err := <-handlerDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("the handler did not return after the client disconnected")
	}
	assert.Equal(t, 0, srv.runHub.subscribers(runID))

	// A transition after the disconnect reaches nobody and panics nothing.
	require.NoError(t, rm.CancelRun(ctx, runID, "late"))
}

// TestHandleAutomationRunSSE_SnapshotClosesSettledRun proves the snapshot
// path: a run that is already settled gets its snapshot, then the next tick
// sends done and the handler returns on its own.
func TestHandleAutomationRunSSE_SnapshotClosesSettledRun(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projectID := seedRunFlowProject(t, srv, "sse-snapshot")
	ctx := context.Background()

	run := &bstore.AutomationRun{ProjectID: projectID, TriggerType: "connector.push.completed", Status: bstore.RunStatusRunning}
	require.NoError(t, srv.AutomationRunStore.CreateRun(ctx, run))
	step := &bstore.AutomationStep{RunID: run.ID, RuleName: "draft", ActionType: "notify", Status: bstore.StepStatusCompleted}
	require.NoError(t, srv.AutomationRunStore.CreateStep(ctx, step))
	require.NoError(t, srv.AutomationRunStore.UpdateRunStatus(ctx, run.ID, bstore.RunStatusCompleted, ""))

	rec, cancel, handlerDone := openRunStream(t, srv, projectID, run.ID)
	defer cancel()

	select {
	case err := <-handlerDone:
		require.NoError(t, err)
	case <-time.After(6 * time.Second):
		t.Fatal("the stream of a settled run did not close on the snapshot tick")
	}
	body := rec.body()
	assert.True(t, strings.HasSuffix(body, "event: done\ndata: {}\n\n"), "body: %q", body)
	frames := runStreamFrames(t, body)
	require.Len(t, frames, 2, "the opening snapshot and the tick's snapshot")
	for _, f := range frames {
		assert.Equal(t, "snapshot", f.Type)
		assert.Equal(t, bstore.RunStatusCompleted, f.Run.Status)
		require.Len(t, f.Steps, 1)
	}
	assert.Equal(t, 0, srv.runHub.subscribers(run.ID))
}

// TestHandleCancelAutomationRun_PushesRunFinished proves a cancel from the
// API reaches the run's subscribers as the run's terminal transition.
func TestHandleCancelAutomationRun_PushesRunFinished(t *testing.T) {
	srv := newRunFlowTestServer(t)
	projectID := seedRunFlowProject(t, srv, "sse-cancel")
	ctx := context.Background()

	run := &bstore.AutomationRun{ProjectID: projectID, TriggerType: "connector.push.completed", Status: bstore.RunStatusRunning}
	require.NoError(t, srv.AutomationRunStore.CreateRun(ctx, run))
	step := &bstore.AutomationStep{RunID: run.ID, RuleName: "draft", ActionType: "auto_translate", Status: bstore.StepStatusRunning}
	require.NoError(t, srv.AutomationRunStore.CreateStep(ctx, step))

	ch := srv.runHub.subscribe(run.ID)
	defer srv.runHub.unsubscribe(run.ID, ch)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/acme/"+projectID+"/automations/runs/"+run.ID+"/cancel", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "runId")
	c.SetParamValues(projectID, run.ID)
	require.NoError(t, srv.HandleCancelAutomationRun(c))
	require.Equal(t, http.StatusOK, rec.Code)

	select {
	case payload := <-ch:
		var f automationRunFrame
		require.NoError(t, json.Unmarshal(payload, &f))
		assert.Equal(t, "run.finished", f.Type)
		assert.Equal(t, bstore.RunStatusFailed, f.Run.Status)
		assert.Equal(t, "cancelled by user", f.Run.Error)
		require.Len(t, f.Steps, 1)
	case <-time.After(2 * time.Second):
		t.Fatal("the cancel was not pushed to the run's subscriber")
	}

	got, err := srv.AutomationRunStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.RunStatusFailed, got.Status)
}

// TestAutomationRunHub_FanOut proves the hub's contract: every subscriber of
// a run receives a transition, an unsubscribed one receives nothing more and
// its channel closes, a full subscriber drops the frame rather than blocking
// the producer, and a change without a run is ignored.
func TestAutomationRunHub_FanOut(t *testing.T) {
	hub := newAutomationRunHub()
	run := &bstore.AutomationRun{ID: "run-1", Status: bstore.RunStatusRunning}
	change := func(kind event.AutomationRunChangeKind) event.AutomationRunChange {
		return event.AutomationRunChange{Kind: kind, Run: run, Steps: nil}
	}

	a := hub.subscribe("run-1")
	b := hub.subscribe("run-1")
	other := hub.subscribe("run-2")
	assert.Equal(t, 2, hub.subscribers("run-1"))

	hub.RunChanged(context.Background(), change(event.AutomationStepStarted))
	for _, ch := range []chan []byte{a, b} {
		select {
		case payload := <-ch:
			var f automationRunFrame
			require.NoError(t, json.Unmarshal(payload, &f))
			assert.Equal(t, "step.started", f.Type)
			assert.Equal(t, "run-1", f.Run.ID)
			assert.NotNil(t, f.Steps, "steps encode as an empty list, never null")
		default:
			t.Fatal("a subscriber missed the transition")
		}
	}
	select {
	case <-other:
		t.Fatal("a subscriber of another run received the transition")
	default:
	}

	hub.unsubscribe("run-1", a)
	_, open := <-a
	assert.False(t, open, "an unsubscribed channel is closed")
	assert.Equal(t, 1, hub.subscribers("run-1"))

	hub.RunChanged(context.Background(), change(event.AutomationStepFinished))
	select {
	case payload := <-b:
		assert.Contains(t, string(payload), `"type":"step.finished"`)
	default:
		t.Fatal("the remaining subscriber missed the transition")
	}

	// A subscriber that never reads fills up; the producer never blocks.
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for range cap(b) + 4 {
			hub.RunChanged(context.Background(), change(event.AutomationStepProgress))
		}
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast blocked on a full subscriber")
	}

	hub.RunChanged(context.Background(), event.AutomationRunChange{Kind: event.AutomationRunFinished})
	hub.unsubscribe("run-1", b)
	hub.unsubscribe("run-2", other)
	assert.Equal(t, 0, hub.subscribers("run-1"))
	assert.Equal(t, 0, hub.subscribers("run-2"))
}
