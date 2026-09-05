package event

import (
	"context"
	"log/slog"

	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// AutomationRunChangeKind names the transition an AutomationRunChange reports.
type AutomationRunChangeKind string

const (
	// AutomationRunStarted reports the run row created for an event.
	AutomationRunStarted AutomationRunChangeKind = "run.started"
	// AutomationStepStarted reports a step created and its action dispatched.
	AutomationStepStarted AutomationRunChangeKind = "step.started"
	// AutomationStepProgress reports a moved completed-job count on a step.
	AutomationStepProgress AutomationRunChangeKind = "step.progress"
	// AutomationStepFinished reports a step at a terminal status, which the
	// step record carries: completed or failed.
	AutomationStepFinished AutomationRunChangeKind = "step.finished"
	// AutomationRunFinished reports the run at a terminal status, which the
	// run record carries: completed, partial or failed.
	AutomationRunFinished AutomationRunChangeKind = "run.finished"
)

// AutomationRunChange is one persisted transition of an automation run,
// carrying the run and its steps as stored after it.
type AutomationRunChange struct {
	Kind  AutomationRunChangeKind
	Run   *bstore.AutomationRun
	Steps []*bstore.AutomationStep
}

// AutomationRunNotifier is told about every transition the run manager and
// the step tracker persist. The server's SSE hub implements it to push the
// record to the run's subscribers.
type AutomationRunNotifier interface {
	RunChanged(ctx context.Context, change AutomationRunChange)
}

// notifyRunChange loads the run's current record and hands it to the
// notifier. A nil notifier, an empty run id or a failed read is a no-op: the
// SSE stream's periodic snapshot carries the state on its next tick.
func notifyRunChange(ctx context.Context, store *bstore.AutomationRunStore, n AutomationRunNotifier, kind AutomationRunChangeKind, runID string) {
	if n == nil || store == nil || runID == "" {
		return
	}
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		slog.Debug("automation run change: load run", "run", runID, "kind", kind, "error", err)
		return
	}
	steps, err := store.ListSteps(ctx, runID)
	if err != nil {
		slog.Debug("automation run change: list steps", "run", runID, "kind", kind, "error", err)
		return
	}
	n.RunChanged(ctx, AutomationRunChange{Kind: kind, Run: run, Steps: steps})
}

// settleAutomationRun marks a run completed, or partial when a step failed,
// once every step has reported, and returns whether it did. A run whose
// steps are still open is left alone.
func settleAutomationRun(ctx context.Context, store *bstore.AutomationRunStore, runID string) bool {
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return false
	}
	if run.StepCount == 0 || run.DoneCount < run.StepCount {
		return false
	}
	steps, err := store.ListSteps(ctx, runID)
	if err != nil {
		return false
	}
	status := bstore.RunStatusCompleted
	for _, s := range steps {
		if s.Status == bstore.StepStatusFailed {
			status = bstore.RunStatusPartial
			break
		}
	}
	if err := store.UpdateRunStatus(ctx, runID, status, ""); err != nil {
		slog.Warn("automation run: failed to settle run", "run", runID, "error", err)
		return false
	}
	return true
}
