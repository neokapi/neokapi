package event

import (
	"context"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runAndStep reads the single run for an event and its single step.
func runAndStep(t *testing.T, store *bstore.AutomationRunStore, evID string) (*bstore.AutomationRun, *bstore.AutomationStep) {
	t.Helper()
	run := runFor(t, store, evID)
	steps, err := store.ListSteps(t.Context(), run.ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	return run, steps[0]
}

// runFor reads the run an event started, whatever number of steps it holds.
func runFor(t *testing.T, store *bstore.AutomationRunStore, evID string) *bstore.AutomationRun {
	t.Helper()
	runs, err := store.ListRuns(t.Context(), "proj-1", "", 20, 0)
	require.NoError(t, err)
	for _, run := range runs {
		if run.TriggerID == evID {
			return run
		}
	}
	t.Fatalf("no run for event %s", evID)
	return nil
}

// TestCancelRun_StopsTheRunningAction proves a cancel reaches the work: the
// action's context is cancelled, the step it was running is closed as skipped
// with the reason, and the run ends cancelled rather than failed.
func TestCancelRun_StopsTheRunningAction(t *testing.T) {
	store := newTestRunStore(t)

	started := make(chan struct{})
	stopped := make(chan error, 1)
	var once sync.Once
	executor := func(ctx context.Context, _ AutomationAction, _ platev.Event, _ string) error {
		go func() {
			once.Do(func() { close(started) })
			<-ctx.Done()
			stopped <- ctx.Err()
		}()
		return nil
	}
	rm := NewAutomationRunManager(store, executor)

	ev := platev.Event{ID: "evt-cancel", Type: platev.EventPullCompleted, ProjectID: "proj-1"}
	require.NoError(t, rm.Execute(AutomationAction{Type: "run_flow", Name: "long-flow"}, ev))
	<-started

	run, step := runAndStep(t, store, ev.ID)
	require.Equal(t, bstore.RunStatusRunning, run.Status)
	require.Equal(t, bstore.StepStatusRunning, step.Status)

	require.NoError(t, rm.CancelRun(t.Context(), run.ID, "cancelled by user"))

	select {
	case err := <-stopped:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("the action was never told to stop")
	}

	run, step = runAndStep(t, store, ev.ID)
	assert.Equal(t, bstore.RunStatusCancelled, run.Status)
	assert.Equal(t, "cancelled by user", run.Error)
	assert.NotNil(t, run.EndedAt)
	assert.Equal(t, bstore.StepStatusSkipped, step.Status)
	assert.Equal(t, "cancelled by user", step.Error)
	assert.NotNil(t, step.EndedAt)
}

// TestCancelRun_LateStepCannotReviveIt proves the record stands: an action that
// closes its step after the cancellation leaves both the step and the run as
// the cancellation left them.
func TestCancelRun_LateStepCannotReviveIt(t *testing.T) {
	store := newTestRunStore(t)
	rm := NewAutomationRunManager(store, func(context.Context, AutomationAction, platev.Event, string) error { return nil })

	ev := platev.Event{ID: "evt-late", Type: platev.EventPullCompleted, ProjectID: "proj-1"}
	require.NoError(t, rm.Execute(AutomationAction{Type: "run_flow", Name: "late-flow"}, ev))
	run, step := runAndStep(t, store, ev.ID)

	require.NoError(t, rm.CancelRun(t.Context(), run.ID, "cancelled by user"))
	rm.CompleteStep(t.Context(), step.ID, nil)

	run, step = runAndStep(t, store, ev.ID)
	assert.Equal(t, bstore.RunStatusCancelled, run.Status)
	assert.Equal(t, bstore.StepStatusSkipped, step.Status)
}

// TestCancelRun_RefusesAFinishedRun proves a run that ran to the end keeps its
// outcome: cancelling it answers ErrRunNotCancellable and writes nothing.
func TestCancelRun_RefusesAFinishedRun(t *testing.T) {
	store := newTestRunStore(t)
	rm := NewAutomationRunManager(store, func(context.Context, AutomationAction, platev.Event, string) error { return nil })

	ev := platev.Event{ID: "evt-done", Type: platev.EventPushCompleted, ProjectID: "proj-1"}
	require.NoError(t, rm.Execute(AutomationAction{Type: "notify", Name: "tell-me"}, ev))
	run, _ := runAndStep(t, store, ev.ID)
	require.Equal(t, bstore.RunStatusCompleted, run.Status)

	require.ErrorIs(t, rm.CancelRun(t.Context(), run.ID, "cancelled by user"), ErrRunNotCancellable)

	run, _ = runAndStep(t, store, ev.ID)
	assert.Equal(t, bstore.RunStatusCompleted, run.Status)
	assert.Empty(t, run.Error)
}

// TestCancelRun_ReportsTheTransition proves the run's stream subscribers are
// told, so a cancel shows up on an open run view without waiting for a
// snapshot tick.
func TestCancelRun_ReportsTheTransition(t *testing.T) {
	store := newTestRunStore(t)
	n := &recordingNotifier{}
	rm := NewAutomationRunManager(store, func(context.Context, AutomationAction, platev.Event, string) error { return nil })
	rm.SetRunNotifier(n)

	ev := platev.Event{ID: "evt-stream", Type: platev.EventPullCompleted, ProjectID: "proj-1"}
	require.NoError(t, rm.Execute(AutomationAction{Type: "run_flow", Name: "streamed"}, ev))
	run, _ := runAndStep(t, store, ev.ID)
	require.NoError(t, rm.CancelRun(t.Context(), run.ID, "cancelled by user"))

	last := n.last()
	require.NotNil(t, last.Run)
	assert.Equal(t, AutomationRunFinished, last.Kind)
	assert.Equal(t, bstore.RunStatusCancelled, last.Run.Status)
}

// TestCancelRun_CancelsTheJobsItsStepsQueued proves the cancellation reaches
// the queue: an auto_translate step's jobs are cancelled, not left to run to
// completion behind a record that says the run stopped.
func TestCancelRun_CancelsTheJobsItsStepsQueued(t *testing.T) {
	store := newTestRunStore(t)
	rm := NewAutomationRunManager(store, func(context.Context, AutomationAction, platev.Event, string) error { return nil })
	canceller := &recordingJobCanceller{}
	rm.SetJobCanceller(canceller)

	ev := platev.Event{ID: "evt-jobs", Type: platev.EventPushCompleted, ProjectID: "proj-1"}
	require.NoError(t, rm.Execute(AutomationAction{Type: "auto_translate", Name: "translate-rule"}, ev))
	run, step := runAndStep(t, store, ev.ID)
	require.NoError(t, store.RegisterStepJobs(t.Context(), step.ID, []string{"job-1", "job-2"}))

	require.NoError(t, rm.CancelRun(t.Context(), run.ID, "cancelled by user"))
	assert.Equal(t, []string{"job-1", "job-2"}, canceller.cancelled())
}

// recordingJobCanceller records the jobs a cancellation reached.
type recordingJobCanceller struct {
	mu   sync.Mutex
	seen []string
}

func (r *recordingJobCanceller) CancelJob(_ context.Context, id, _ string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, id)
	return true, nil
}

func (r *recordingJobCanceller) cancelled() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

// TestCancelRun_CancelsTheExtractionJobsItsStepsQueued proves the cancellation
// reaches the extraction queue too: an auto_extract step's jobs were left to
// run to the end, creating the items the run was stopped for.
func TestCancelRun_CancelsTheExtractionJobsItsStepsQueued(t *testing.T) {
	store := newTestRunStore(t)
	rm := NewAutomationRunManager(store, func(context.Context, AutomationAction, platev.Event, string) error { return nil })
	translations := &recordingJobCanceller{}
	extractions := &recordingExtractionCanceller{}
	rm.SetJobCanceller(translations)
	rm.SetExtractionJobCanceller(extractions)

	ev := platev.Event{ID: "evt-extract", Type: platev.EventPushCompleted, ProjectID: "proj-1"}
	require.NoError(t, rm.Execute(AutomationAction{Type: "auto_extract", Name: "extract-rule"}, ev))
	run, step := runAndStep(t, store, ev.ID)
	require.NoError(t, store.RegisterStepJobs(t.Context(), step.ID, []string{"ext-1", "ext-2"}))

	require.NoError(t, rm.CancelRun(t.Context(), run.ID, "cancelled by user"))
	assert.Equal(t, []string{"ext-1", "ext-2"}, extractions.cancelled())
	assert.Empty(t, translations.cancelled(), "an extraction id reached the translation queue")
}

// TestCancelRun_SendsEachStepsJobsToItsOwnQueue proves the routing is by action
// type: a translate step's ids never reach the extraction store, which would
// cancel nothing and leave the real jobs running.
func TestCancelRun_SendsEachStepsJobsToItsOwnQueue(t *testing.T) {
	store := newTestRunStore(t)
	rm := NewAutomationRunManager(store, func(context.Context, AutomationAction, platev.Event, string) error { return nil })
	translations := &recordingJobCanceller{}
	extractions := &recordingExtractionCanceller{}
	rm.SetJobCanceller(translations)
	rm.SetExtractionJobCanceller(extractions)

	ev := platev.Event{ID: "evt-both", Type: platev.EventPushCompleted, ProjectID: "proj-1"}
	require.NoError(t, rm.Execute(AutomationAction{Type: "auto_translate", Name: "translate-rule"}, ev))
	require.NoError(t, rm.Execute(AutomationAction{Type: "auto_extract", Name: "extract-rule"}, ev))

	run := runFor(t, store, ev.ID)
	steps, err := store.ListSteps(t.Context(), run.ID)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	for _, step := range steps {
		switch step.ActionType {
		case "auto_translate":
			require.NoError(t, store.RegisterStepJobs(t.Context(), step.ID, []string{"job-1"}))
		case "auto_extract":
			require.NoError(t, store.RegisterStepJobs(t.Context(), step.ID, []string{"ext-1"}))
		}
	}

	require.NoError(t, rm.CancelRun(t.Context(), run.ID, "cancelled by user"))
	assert.Equal(t, []string{"job-1"}, translations.cancelled())
	assert.Equal(t, []string{"ext-1"}, extractions.cancelled())
}

// recordingExtractionCanceller records the extraction jobs a cancellation
// reached.
type recordingExtractionCanceller struct {
	mu   sync.Mutex
	seen []string
}

func (r *recordingExtractionCanceller) CancelExtractionJob(_ context.Context, id, _ string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, id)
	return true, nil
}

func (r *recordingExtractionCanceller) cancelled() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}
