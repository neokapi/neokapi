package event

import (
	"context"
	"log/slog"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
)

// StepAwareExecutor is an action executor that receives the step ID for tracking.
type StepAwareExecutor func(action AutomationAction, event platev.Event, stepID string) error

// AutomationRunManager wraps the action executor, creating runs and steps
// as side effects. It groups actions from the same event into a single run.
type AutomationRunManager struct {
	store    *bstore.AutomationRunStore
	executor StepAwareExecutor

	mu          sync.Mutex
	eventRuns   map[string]string      // event.ID → run.ID (active window)
	cleanTimers map[string]*time.Timer // event.ID → cleanup timer
}

// NewAutomationRunManager creates a manager that tracks automation runs.
// If store is nil, the manager passes through to the executor without tracking.
func NewAutomationRunManager(store *bstore.AutomationRunStore, executor StepAwareExecutor) *AutomationRunManager {
	return &AutomationRunManager{
		store:       store,
		executor:    executor,
		eventRuns:   make(map[string]string),
		cleanTimers: make(map[string]*time.Timer),
	}
}

// Stop cancels all pending cleanup timers. Call during shutdown.
func (m *AutomationRunManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for evID, t := range m.cleanTimers {
		t.Stop()
		delete(m.cleanTimers, evID)
	}
}

// Execute is called by the AutomationEngine for each matching action.
// It creates a run (or reuses one for the same event) and a step, then
// delegates to the real executor.
func (m *AutomationRunManager) Execute(action AutomationAction, ev platev.Event) error {
	if m.store == nil {
		// No tracking — pass through with empty step ID.
		return m.executor(action, ev, "")
	}

	ctx := context.Background()

	// Find or create run for this event.
	runID := m.getOrCreateRun(ctx, ev)

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

	// Execute the real action with step ID.
	err := m.executor(action, ev, step.ID)

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
		m.maybeCompleteRun(ctx, runID)
	}

	return err
}

// getOrCreateRun finds an existing run for the event or creates a new one.
func (m *AutomationRunManager) getOrCreateRun(ctx context.Context, ev platev.Event) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if runID, ok := m.eventRuns[ev.ID]; ok {
		return runID
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

	// Clean up the mapping after a debounce window (events arrive in quick succession).
	// Use time.AfterFunc instead of a goroutine+sleep to allow cancellation on shutdown.
	evID := ev.ID
	m.cleanTimers[evID] = time.AfterFunc(5*time.Second, func() {
		m.mu.Lock()
		delete(m.eventRuns, evID)
		delete(m.cleanTimers, evID)
		m.mu.Unlock()
	})

	return run.ID
}

// maybeCompleteRun checks if all steps are done and updates run status.
func (m *AutomationRunManager) maybeCompleteRun(ctx context.Context, runID string) {
	run, err := m.store.GetRun(ctx, runID)
	if err != nil {
		return
	}
	if run.DoneCount >= run.StepCount && run.StepCount > 0 {
		// Check if any step failed.
		steps, err := m.store.ListSteps(ctx, runID)
		if err != nil {
			return
		}
		status := bstore.RunStatusCompleted
		for _, s := range steps {
			if s.Status == bstore.StepStatusFailed {
				status = bstore.RunStatusPartial
				break
			}
		}
		_ = m.store.UpdateRunStatus(ctx, runID, status, "")
	}
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
	m.maybeCompleteRun(ctx, step.RunID)
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
