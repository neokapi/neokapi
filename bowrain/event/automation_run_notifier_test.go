package event

import (
	"context"
	"errors"
	"sync"
	"testing"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingNotifier keeps every change it is told about, in order.
type recordingNotifier struct {
	mu      sync.Mutex
	changes []AutomationRunChange
}

func (r *recordingNotifier) RunChanged(_ context.Context, c AutomationRunChange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changes = append(r.changes, c)
}

func (r *recordingNotifier) kinds() []AutomationRunChangeKind {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AutomationRunChangeKind, 0, len(r.changes))
	for _, c := range r.changes {
		out = append(out, c.Kind)
	}
	return out
}

// last is the latest change; a test asserts kinds() first, so it is never
// called on an empty recorder.
func (r *recordingNotifier) last() AutomationRunChange {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.changes[len(r.changes)-1]
}

func (r *recordingNotifier) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changes = nil
}

// TestRunManager_ReportsEachTransition proves a synchronous action's run is
// reported at every persisted step, each report carrying the record as
// stored: the run created, the step opened, the step closed, the run
// settled. A second action for the same event joins the run rather than
// starting another.
func TestRunManager_ReportsEachTransition(t *testing.T) {
	store := newTestRunStore(t)
	n := &recordingNotifier{}
	rm := NewAutomationRunManager(store, func(AutomationAction, platev.Event, string) error { return nil })
	rm.SetRunNotifier(n)

	ev := platev.Event{ID: "evt-notify", Type: platev.EventPushCompleted, ProjectID: "proj-1"}
	require.NoError(t, rm.Execute(AutomationAction{Type: "notify", Name: "tell-me"}, ev))

	require.Equal(t, []AutomationRunChangeKind{
		AutomationRunStarted, AutomationStepStarted, AutomationStepFinished, AutomationRunFinished,
	}, n.kinds())

	started := n.changes[0]
	assert.Equal(t, bstore.RunStatusRunning, started.Run.Status)
	assert.Empty(t, started.Steps, "the run is reported before its first step exists")

	opened := n.changes[1]
	require.Len(t, opened.Steps, 1)
	assert.Equal(t, bstore.StepStatusRunning, opened.Steps[0].Status)
	assert.Equal(t, "tell-me", opened.Steps[0].RuleName)

	closed := n.changes[2]
	require.Len(t, closed.Steps, 1)
	assert.Equal(t, bstore.StepStatusCompleted, closed.Steps[0].Status)
	assert.Equal(t, 1, closed.Run.DoneCount)
	assert.Equal(t, bstore.RunStatusRunning, closed.Run.Status, "the step closes before the run settles")

	settled := n.changes[3]
	assert.Equal(t, bstore.RunStatusCompleted, settled.Run.Status)
	assert.NotNil(t, settled.Run.EndedAt)

	// The same event's next action reuses the run: no second run.started.
	n.reset()
	require.NoError(t, rm.Execute(AutomationAction{Type: "notify", Name: "tell-me-too"}, ev))
	require.Equal(t, []AutomationRunChangeKind{
		AutomationStepStarted, AutomationStepFinished, AutomationRunFinished,
	}, n.kinds())
	assert.Equal(t, started.Run.ID, n.last().Run.ID)
	assert.Len(t, n.last().Steps, 2)
}

// TestRunManager_NoNotifierIsSilent proves the manager records runs as before
// when nothing listens.
func TestRunManager_NoNotifierIsSilent(t *testing.T) {
	store := newTestRunStore(t)
	rm := NewAutomationRunManager(store, func(AutomationAction, platev.Event, string) error { return nil })
	require.NoError(t, rm.Execute(AutomationAction{Type: "notify", Name: "quiet"},
		platev.Event{ID: "evt-quiet", ProjectID: "proj-1"}))
	runs, err := store.ListRuns(t.Context(), "proj-1", "", 10, 0)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, bstore.RunStatusCompleted, runs[0].Status)
}

// TestRunManager_CompleteStep_ReportsTransitions proves a self-reporting
// action's step is reported when it opens and again when the action closes
// it, with the run settling on the outcome.
func TestRunManager_CompleteStep_ReportsTransitions(t *testing.T) {
	store := newTestRunStore(t)
	ctx := t.Context()
	n := &recordingNotifier{}
	rm := NewAutomationRunManager(store, func(AutomationAction, platev.Event, string) error { return nil })
	rm.SetRunNotifier(n)

	require.NoError(t, rm.Execute(
		AutomationAction{Type: "run_flow", Name: "checks", Config: map[string]string{"flow": "qa"}},
		platev.Event{ID: "evt-flow", Type: platev.EventPullCompleted, ProjectID: "proj-1"}))
	require.Equal(t, []AutomationRunChangeKind{AutomationRunStarted, AutomationStepStarted}, n.kinds(),
		"a self-reporting step stays open when Execute returns")
	step := n.last().Steps[0]

	n.reset()
	rm.CompleteStep(ctx, step.ID, errors.New("tool failed"))
	require.Equal(t, []AutomationRunChangeKind{AutomationStepFinished, AutomationRunFinished}, n.kinds())
	assert.Equal(t, bstore.StepStatusFailed, n.changes[0].Steps[0].Status)
	assert.Equal(t, "tool failed", n.changes[0].Steps[0].Error)
	assert.Equal(t, bstore.RunStatusPartial, n.last().Run.Status)
}

// TestRunManager_AsyncDispatchFailureSettlesRun proves a job-spawning action
// that fails at dispatch closes its run: the failed step is the run's only
// step, so the run settles as partial rather than staying running with
// nothing left to finish it.
func TestRunManager_AsyncDispatchFailureSettlesRun(t *testing.T) {
	store := newTestRunStore(t)
	n := &recordingNotifier{}
	rm := NewAutomationRunManager(store, func(AutomationAction, platev.Event, string) error {
		return errors.New("queue unavailable")
	})
	rm.SetRunNotifier(n)

	err := rm.Execute(AutomationAction{Type: "auto_translate", Name: "draft"},
		platev.Event{ID: "evt-async-fail", ProjectID: "proj-1"})
	require.Error(t, err)
	require.Equal(t, []AutomationRunChangeKind{
		AutomationRunStarted, AutomationStepStarted, AutomationStepFinished, AutomationRunFinished,
	}, n.kinds())
	assert.Equal(t, bstore.StepStatusFailed, n.last().Steps[0].Status)
	assert.Equal(t, bstore.RunStatusPartial, n.last().Run.Status)
}

// TestRunManager_CancelRun proves a cancel is a reported transition: the run
// is marked failed with the reason and its subscribers hear of it.
func TestRunManager_CancelRun(t *testing.T) {
	store := newTestRunStore(t)
	ctx := t.Context()
	n := &recordingNotifier{}
	rm := NewAutomationRunManager(store, func(AutomationAction, platev.Event, string) error { return nil })
	rm.SetRunNotifier(n)

	require.NoError(t, rm.Execute(AutomationAction{Type: "auto_translate", Name: "draft"},
		platev.Event{ID: "evt-cancel", ProjectID: "proj-1"}))
	runID := n.last().Run.ID
	n.reset()

	require.NoError(t, rm.CancelRun(ctx, runID, "cancelled by user"))
	require.Equal(t, []AutomationRunChangeKind{AutomationRunFinished}, n.kinds())
	assert.Equal(t, bstore.RunStatusFailed, n.last().Run.Status)
	assert.Equal(t, "cancelled by user", n.last().Run.Error)
	assert.NotNil(t, n.last().Run.EndedAt)

	run, err := store.GetRun(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, bstore.RunStatusFailed, run.Status)

	// Nothing to cancel is a no-op rather than an error.
	require.NoError(t, rm.CancelRun(ctx, "", "x"))
	require.NoError(t, NewAutomationRunManager(nil, nil).CancelRun(ctx, runID, "x"))
}

// statusJobStore answers GetJob with the status recorded per id.
type statusJobStore struct {
	stubJobStore
	statuses map[string]jobs.JobStatus
}

func (s *statusJobStore) GetJob(_ context.Context, id string) (*jobs.TranslationJob, error) {
	st, ok := s.statuses[id]
	if !ok {
		return nil, errors.New("no such job")
	}
	return &jobs.TranslationJob{ID: id, Status: st}, nil
}

// seedTrackedStep creates a running run with one running auto_translate step
// that spawned the given jobs, and returns the run and step ids.
func seedTrackedStep(t *testing.T, store *bstore.AutomationRunStore, jobIDs []string) (runID, stepID string) {
	t.Helper()
	ctx := t.Context()
	run := &bstore.AutomationRun{ProjectID: "proj-1", TriggerType: "connector.push.completed", Status: bstore.RunStatusRunning}
	require.NoError(t, store.CreateRun(ctx, run))
	step := &bstore.AutomationStep{RunID: run.ID, RuleName: "draft", ActionType: "auto_translate", Status: bstore.StepStatusRunning}
	require.NoError(t, store.CreateStep(ctx, step))
	require.NoError(t, store.RegisterStepJobs(ctx, step.ID, jobIDs))
	return run.ID, step.ID
}

func newCheckingTracker(store *bstore.AutomationRunStore, js jobs.JobStore, n AutomationRunNotifier) *StepCompletionTracker {
	tr := &StepCompletionTracker{
		runStore: store,
		jobStore: js,
		pending:  make(map[string]*pendingStep),
		done:     make(chan struct{}),
	}
	tr.SetRunNotifier(n)
	return tr
}

// TestStepTracker_ClosesStepAndSettlesRunFromStepRecord proves a step
// registered without its run id (the way the executor registers it) still
// credits and settles its run: the step record names the run. The tracker
// reports the step closing and the run settling, and stops polling the step.
func TestStepTracker_ClosesStepAndSettlesRunFromStepRecord(t *testing.T) {
	store := newTestRunStore(t)
	ctx := t.Context()
	runID, stepID := seedTrackedStep(t, store, []string{"job-1", "job-2"})
	n := &recordingNotifier{}
	tr := newCheckingTracker(store, &statusJobStore{statuses: map[string]jobs.JobStatus{
		"job-1": jobs.StatusCompleted, "job-2": jobs.StatusCompleted,
	}}, n)

	tr.TrackStep(stepID, "", false)
	tr.checkPending()

	require.Equal(t, []AutomationRunChangeKind{AutomationStepFinished, AutomationRunFinished}, n.kinds())
	assert.Equal(t, runID, n.last().Run.ID)
	assert.Equal(t, bstore.RunStatusCompleted, n.last().Run.Status)
	assert.Equal(t, 1, n.last().Run.DoneCount)
	require.Len(t, n.last().Steps, 1)
	assert.Equal(t, bstore.StepStatusCompleted, n.last().Steps[0].Status)
	assert.Equal(t, 2, n.last().Steps[0].DoneJobs)

	run, err := store.GetRun(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, bstore.RunStatusCompleted, run.Status)

	tr.mu.Lock()
	_, stillPending := tr.pending[stepID]
	tr.mu.Unlock()
	assert.False(t, stillPending, "a closed step is not polled again")

	// A further check finds nothing to do and reports nothing.
	n.reset()
	tr.checkPending()
	assert.Empty(t, n.kinds())
}

// TestStepTracker_ReportsJobProgress proves a moved job count is reported
// while the step stays open, and that the step is still tracked.
func TestStepTracker_ReportsJobProgress(t *testing.T) {
	store := newTestRunStore(t)
	_, stepID := seedTrackedStep(t, store, []string{"job-1", "job-2"})
	n := &recordingNotifier{}
	tr := newCheckingTracker(store, &statusJobStore{statuses: map[string]jobs.JobStatus{
		"job-1": jobs.StatusCompleted, "job-2": jobs.StatusQueued,
	}}, n)

	tr.TrackStep(stepID, "", false)
	tr.checkPending()

	require.Equal(t, []AutomationRunChangeKind{AutomationStepProgress}, n.kinds())
	require.Len(t, n.last().Steps, 1)
	assert.Equal(t, 1, n.last().Steps[0].DoneJobs)
	assert.Equal(t, 2, n.last().Steps[0].TotalJobs)
	assert.Equal(t, bstore.StepStatusRunning, n.last().Steps[0].Status)
	assert.Equal(t, bstore.RunStatusRunning, n.last().Run.Status)

	tr.mu.Lock()
	_, stillPending := tr.pending[stepID]
	tr.mu.Unlock()
	assert.True(t, stillPending)

	// An unchanged count on the next check is not reported again.
	n.reset()
	tr.checkPending()
	assert.Empty(t, n.kinds())
}
