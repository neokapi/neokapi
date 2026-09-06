package event

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
)

// ErrRunNotCancellable reports a cancel asked of a run that has already
// finished. Its work is over, and writing "cancelled" over the outcome would
// say the run stopped when it ran to the end.
var ErrRunNotCancellable = errors.New("automation run has already finished")

// JobCanceller stops a translation job a person asked to stop. It is the half
// of jobs.JobStore a cancellation needs, so the run manager depends on the
// verb rather than on the whole store.
type JobCanceller interface {
	CancelJob(ctx context.Context, id, reason string) (cancelled bool, err error)
}

// StepAwareExecutor is an action executor that receives the run's context and
// the step ID for tracking.
//
// The context is the run's: cancelling the run cancels it, so an action that
// threads it into its work stops rather than running on past a record that
// says it was stopped.
type StepAwareExecutor func(ctx context.Context, action AutomationAction, event platev.Event, stepID string) error

// runExecution is the context one run's actions execute under, and what the
// manager has to know before it can let go of it.
type runExecution struct {
	ctx    context.Context
	cancel context.CancelFunc
	// windowOpen is true while the event's grouping window has not closed, so
	// the run may still gain a step that needs this context.
	windowOpen bool
	// terminal is true once the run has settled or been cancelled.
	terminal bool
}

// AutomationRunManager wraps the action executor, creating runs and steps
// as side effects. It groups actions from the same event into a single run,
// and reports every transition it persists to its notifier.
type AutomationRunManager struct {
	store    *bstore.AutomationRunStore
	executor StepAwareExecutor
	notifier AutomationRunNotifier
	jobs     JobCanceller

	mu          sync.Mutex
	eventRuns   map[string]string        // event.ID → run.ID (active window)
	cleanTimers map[string]*time.Timer   // event.ID → cleanup timer
	running     map[string]*runExecution // run.ID → the context its actions run under
}

// NewAutomationRunManager creates a manager that tracks automation runs.
// If store is nil, the manager passes through to the executor without tracking.
func NewAutomationRunManager(store *bstore.AutomationRunStore, executor StepAwareExecutor) *AutomationRunManager {
	return &AutomationRunManager{
		store:       store,
		executor:    executor,
		eventRuns:   make(map[string]string),
		cleanTimers: make(map[string]*time.Timer),
		running:     make(map[string]*runExecution),
	}
}

// SetJobCanceller gives the manager a way to stop the translation jobs a
// step queued, so cancelling a run reaches the queue rather than leaving those
// jobs to run to completion behind a record that says the run stopped.
// jobs.JobStore satisfies it. Optional: without one, a cancel still stops
// everything that runs in this process.
func (m *AutomationRunManager) SetJobCanceller(c JobCanceller) {
	m.jobs = c
}

// SetRunNotifier sets the notifier told about every run transition the
// manager persists. Set it before the engine starts dispatching.
func (m *AutomationRunManager) SetRunNotifier(n AutomationRunNotifier) {
	m.notifier = n
}

// Stop cancels all pending cleanup timers and releases every open run's
// context. Call during shutdown, after the actions have been waited on: a run
// still executing when this runs is one the shutdown budget already gave up
// on.
func (m *AutomationRunManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for evID, t := range m.cleanTimers {
		t.Stop()
		delete(m.cleanTimers, evID)
	}
	for runID, rx := range m.running {
		rx.cancel()
		delete(m.running, runID)
	}
}

// Execute is called by the AutomationEngine for each matching action.
// It creates a run (or reuses one for the same event) and a step, then
// delegates to the real executor.
func (m *AutomationRunManager) Execute(action AutomationAction, ev platev.Event) error {
	if m.store == nil {
		// No tracking — pass through with empty step ID. Nothing holds a run
		// to cancel, so the action runs under a background context.
		return m.executor(context.Background(), action, ev, "")
	}

	ctx := context.Background()

	// Find or create run for this event.
	runID, created := m.getOrCreateRun(ctx, ev)
	if created {
		m.notify(ctx, AutomationRunStarted, runID)
	}

	// Create step.
	step := &bstore.AutomationStep{
		ID:         id.New(),
		RunID:      runID,
		RuleName:   action.Name,
		ActionType: action.Type,
		Status:     bstore.StepStatusRunning,
		Config:     action.Config,
	}
	if err := m.store.CreateStep(ctx, step); err != nil {
		slog.Info("run-manager: failed to create step", "error", err)
	}

	_ = m.store.AppendLogs(ctx, []bstore.AutomationLog{{
		StepID:  step.ID,
		RunID:   runID,
		Level:   "info",
		Message: "Starting action: " + action.Type,
	}})
	m.notify(ctx, AutomationStepStarted, runID)

	// Execute the real action under the run's context, so a cancellation
	// reaches the work rather than only the record.
	err := m.executor(m.runContext(runID), action, ev, step.ID)

	// For synchronous actions (create_review_tasks, create_source_review, notify),
	// mark step completed immediately. Async actions (auto_translate, auto_extract)
	// stay "running" until StepCompletionTracker detects job completion.
	if isAsyncAction(action.Type) {
		// Step stays running — tracked by StepCompletionTracker.
		if err != nil {
			_ = m.store.UpdateStepStatus(ctx, step.ID, bstore.StepStatusFailed, err.Error())
			_ = m.store.IncrementDoneCount(ctx, runID)
			_ = m.store.AppendLogs(ctx, []bstore.AutomationLog{{
				StepID: step.ID, RunID: runID, Level: "error",
				Message: "Action failed: " + err.Error(),
			}})
			m.notify(ctx, AutomationStepFinished, runID)
			m.settle(ctx, runID)
		}
	} else {
		// Synchronous — mark done now.
		if err != nil {
			_ = m.store.UpdateStepStatus(ctx, step.ID, bstore.StepStatusFailed, err.Error())
			_ = m.store.AppendLogs(ctx, []bstore.AutomationLog{{
				StepID: step.ID, RunID: runID, Level: "error",
				Message: "Action failed: " + err.Error(),
			}})
		} else {
			_ = m.store.UpdateStepStatus(ctx, step.ID, bstore.StepStatusCompleted, "")
			_ = m.store.AppendLogs(ctx, []bstore.AutomationLog{{
				StepID: step.ID, RunID: runID, Level: "info",
				Message: "Action completed: " + action.Type,
			}})
		}
		_ = m.store.IncrementDoneCount(ctx, runID)
		m.notify(ctx, AutomationStepFinished, runID)
		m.settle(ctx, runID)
	}

	return err
}

// getOrCreateRun finds an existing run for the event or creates a new one,
// reporting whether it created it.
func (m *AutomationRunManager) getOrCreateRun(ctx context.Context, ev platev.Event) (runID string, created bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if runID, ok := m.eventRuns[ev.ID]; ok {
		return runID, false
	}

	run := &bstore.AutomationRun{
		ID:          id.New(),
		ProjectID:   ev.ProjectID,
		TriggerType: string(ev.Type),
		TriggerID:   ev.ID,
		TriggerData: ev.Data,
		Status:      bstore.RunStatusRunning,
	}
	if err := m.store.CreateRun(ctx, run); err != nil {
		slog.Info("run-manager: failed to create run", "error", err)
	}

	m.eventRuns[ev.ID] = run.ID
	runCtx, cancel := context.WithCancel(context.Background())
	m.running[run.ID] = &runExecution{ctx: runCtx, cancel: cancel, windowOpen: true}

	// Clean up the mapping after a debounce window (events arrive in quick succession).
	// Use time.AfterFunc instead of a goroutine+sleep to allow cancellation on shutdown.
	evID, runID := ev.ID, run.ID
	m.cleanTimers[evID] = time.AfterFunc(5*time.Second, func() {
		m.mu.Lock()
		delete(m.eventRuns, evID)
		delete(m.cleanTimers, evID)
		if rx, ok := m.running[runID]; ok {
			rx.windowOpen = false
			m.releaseLocked(runID, rx)
		}
		m.mu.Unlock()
	})

	return run.ID, true
}

// runContext is the context this run's actions execute under. A run the
// manager no longer holds runs under a background context, which is what an
// untracked pass-through does.
func (m *AutomationRunManager) runContext(runID string) context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rx, ok := m.running[runID]; ok {
		return rx.ctx
	}
	return context.Background()
}

// releaseLocked forgets a run's context once the run has finished and its
// event's grouping window has closed. Both conditions matter: a run that
// settled inside its window can still gain a step, and that step needs a
// context to run under. The caller holds m.mu.
func (m *AutomationRunManager) releaseLocked(runID string, rx *runExecution) {
	if !rx.terminal || rx.windowOpen {
		return
	}
	rx.cancel()
	delete(m.running, runID)
}

// settle closes the run once every step has reported, and reports that.
func (m *AutomationRunManager) settle(ctx context.Context, runID string) {
	if !settleAutomationRun(ctx, m.store, runID) {
		return
	}
	m.mu.Lock()
	if rx, ok := m.running[runID]; ok {
		rx.terminal = true
		m.releaseLocked(runID, rx)
	}
	m.mu.Unlock()
	m.notify(ctx, AutomationRunFinished, runID)
}

// notify hands the run's current record to the notifier.
func (m *AutomationRunManager) notify(ctx context.Context, kind AutomationRunChangeKind, runID string) {
	notifyRunChange(ctx, m.store, m.notifier, kind, runID)
}

// CompleteStep records the outcome of an action that finished after Execute
// returned: the step's status and error, a closing log line, the run's done
// count, and the run's own status once every step has reported. It is the
// counterpart of the synchronous branch in Execute for the actions
// isAsyncAction lists that report their own completion, such as run_flow.
// A nil store makes it a no-op, matching Execute's pass-through.
func (m *AutomationRunManager) CompleteStep(ctx context.Context, stepID string, actionErr error) {
	if m.store == nil || stepID == "" {
		return
	}
	step, err := m.store.GetStep(ctx, stepID)
	if err != nil {
		slog.Warn("run-manager: failed to load step for completion", "step", stepID, "error", err)
		return
	}
	if bstore.StepIsTerminal(step.Status) {
		// The step already reported: a cancellation closed it, or the action
		// closed it twice. Writing again would overwrite what stopped it and
		// count it towards the run a second time.
		return
	}
	if actionErr != nil {
		_ = m.store.UpdateStepStatus(ctx, step.ID, bstore.StepStatusFailed, actionErr.Error())
		_ = m.store.AppendLogs(ctx, []bstore.AutomationLog{{
			StepID: step.ID, RunID: step.RunID, Level: "error",
			Message: "Action failed: " + actionErr.Error(),
		}})
	} else {
		_ = m.store.UpdateStepStatus(ctx, step.ID, bstore.StepStatusCompleted, "")
		_ = m.store.AppendLogs(ctx, []bstore.AutomationLog{{
			StepID: step.ID, RunID: step.RunID, Level: "info",
			Message: "Action completed: " + step.ActionType,
		}})
	}
	_ = m.store.IncrementDoneCount(ctx, step.RunID)
	m.notify(ctx, AutomationStepFinished, step.RunID)
	m.settle(ctx, step.RunID)
}

// CancelRun stops a run and records that it was stopped.
//
// The work stops first: the run's context is cancelled, which is what an
// action observes, and the translation jobs its open steps queued are
// cancelled in the queue. Then the record is closed — each open step skipped
// with the reason, the run itself cancelled — so a reader sees what happened
// to each step rather than a run that merely stopped moving.
//
// A run that has already finished answers ErrRunNotCancellable: its work is
// over, and calling that a cancellation would be a lie the run history then
// repeats. A nil store makes it a no-op, matching Execute's pass-through.
func (m *AutomationRunManager) CancelRun(ctx context.Context, runID, reason string) error {
	if m.store == nil || runID == "" {
		return nil
	}
	run, err := m.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if bstore.RunIsTerminal(run.Status) {
		return ErrRunNotCancellable
	}

	m.stopRun(runID)

	steps, err := m.store.ListSteps(ctx, runID)
	if err != nil {
		slog.WarnContext(ctx, "run-manager: failed to list steps to cancel", "run", runID, "error", err)
	}
	for _, step := range steps {
		if bstore.StepIsTerminal(step.Status) {
			continue
		}
		m.cancelStepJobs(ctx, step, reason)
		if err := m.store.UpdateStepStatus(ctx, step.ID, bstore.StepStatusSkipped, reason); err != nil {
			slog.WarnContext(ctx, "run-manager: failed to close a cancelled step", "step", step.ID, "error", err)
			continue
		}
		_ = m.store.IncrementDoneCount(ctx, runID)
		_ = m.store.AppendLogs(ctx, []bstore.AutomationLog{{
			StepID: step.ID, RunID: runID, Level: "info",
			Message: "Action stopped: " + reason,
		}})
	}

	if err := m.store.UpdateRunStatus(ctx, runID, bstore.RunStatusCancelled, reason); err != nil {
		return err
	}
	m.notify(ctx, AutomationRunFinished, runID)
	return nil
}

// stopRun cancels the context the run's actions execute under. The entry stays
// until releaseLocked can drop it, so an action dispatched into a cancelled
// run inside its grouping window is handed the cancelled context rather than a
// fresh one.
func (m *AutomationRunManager) stopRun(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rx, ok := m.running[runID]
	if !ok {
		return
	}
	rx.terminal = true
	rx.cancel()
	m.releaseLocked(runID, rx)
}

// cancelStepJobs stops the translation jobs a step queued, so a cancellation
// reaches the queue and not only the process that dispatched it. A job that
// has already finished is left alone, which the store reports rather than
// treating as an error.
func (m *AutomationRunManager) cancelStepJobs(ctx context.Context, step *bstore.AutomationStep, reason string) {
	if m.jobs == nil || !spawnsTranslationJobs(step.ActionType) {
		return
	}
	for _, jobID := range step.JobIDs {
		if _, err := m.jobs.CancelJob(ctx, jobID, reason); err != nil {
			slog.WarnContext(ctx, "run-manager: failed to cancel a step's job",
				"step", step.ID, "job", jobID, "error", err)
		}
	}
}

// spawnsTranslationJobs names the actions whose steps carry translation job
// ids. auto_extract queues extraction jobs, which the extraction store has no
// cancel for.
func spawnsTranslationJobs(actionType string) bool {
	switch actionType {
	case "auto_translate", "auto_translate_new_locale":
		return true
	default:
		return false
	}
}

// isAsyncAction returns true for actions whose work outlives Execute: the
// job-spawning actions the StepCompletionTracker closes, and run_flow, which
// closes its own step through CompleteStep when the flow finishes.
func isAsyncAction(actionType string) bool {
	switch actionType {
	case "auto_translate", "auto_extract", "auto_translate_new_locale", "run_flow":
		return true
	default:
		return false
	}
}
