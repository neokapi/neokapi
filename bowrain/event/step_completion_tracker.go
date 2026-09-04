package event

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/observe"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// StepCompletionTracker monitors async automation steps (auto_translate,
// auto_extract) and updates their status when all spawned jobs complete.
// Every transition it persists is reported to its notifier.
type StepCompletionTracker struct {
	runStore     *bstore.AutomationRunStore
	jobStore     jobs.JobStore
	extractStore jobs.ExtractionJobStore
	quotaStore   *jobs.QuotaStoreDB    // optional; nil disables runner usage recording
	billingHooks *billing.UsageHooks   // optional; nil disables billing credit deduction
	notifier     AutomationRunNotifier // optional; nil reports no transitions

	mu           sync.Mutex
	pending      map[string]*pendingStep // stepID → state
	pollInterval time.Duration
	done         chan struct{}
}

type pendingStep struct {
	runID        string
	isExtraction bool
	workspaceID  string
	projectID    string
	actionType   string // e.g. "auto_translate", "auto_extract"
	registeredAt time.Time
}

// NewStepCompletionTracker creates a tracker that polls for step completion.
func NewStepCompletionTracker(
	runStore *bstore.AutomationRunStore,
	jobStore jobs.JobStore,
	extractStore jobs.ExtractionJobStore,
) *StepCompletionTracker {
	t := &StepCompletionTracker{
		runStore:     runStore,
		jobStore:     jobStore,
		extractStore: extractStore,
		pending:      make(map[string]*pendingStep),
		pollInterval: 5 * time.Second,
		done:         make(chan struct{}),
	}
	go t.pollLoop()
	return t
}

// Close stops the tracker.
func (t *StepCompletionTracker) Close() {
	close(t.done)
}

// SetBillingHooks configures billing integration for runner time deduction.
func (t *StepCompletionTracker) SetBillingHooks(hooks *billing.UsageHooks) {
	t.billingHooks = hooks
}

// SetQuotaStore configures runner usage recording.
func (t *StepCompletionTracker) SetQuotaStore(store *jobs.QuotaStoreDB) {
	t.quotaStore = store
}

// SetRunNotifier sets the notifier told about every step and run transition
// the tracker persists. Set it before the first step is tracked.
func (t *StepCompletionTracker) SetRunNotifier(n AutomationRunNotifier) {
	t.notifier = n
}

// StepTrackingInfo carries context for billing when a step completes.
type StepTrackingInfo struct {
	StepID      string
	RunID       string
	Extraction  bool
	WorkspaceID string
	ProjectID   string
	ActionType  string
}

// TrackStep registers a step for completion tracking.
func (t *StepCompletionTracker) TrackStep(stepID, runID string, isExtraction bool) {
	t.TrackStepWithInfo(StepTrackingInfo{
		StepID: stepID, RunID: runID, Extraction: isExtraction,
	})
}

// TrackStepWithInfo registers a step with full billing context.
func (t *StepCompletionTracker) TrackStepWithInfo(info StepTrackingInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[info.StepID] = &pendingStep{
		runID:        info.RunID,
		isExtraction: info.Extraction,
		workspaceID:  info.WorkspaceID,
		projectID:    info.ProjectID,
		actionType:   info.ActionType,
		registeredAt: time.Now(),
	}
}

func (t *StepCompletionTracker) pollLoop() {
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			t.checkPending()
		}
	}
}

func (t *StepCompletionTracker) checkPending() {
	t.mu.Lock()
	stepIDs := make([]string, 0, len(t.pending))
	for id := range t.pending {
		stepIDs = append(stepIDs, id)
	}
	t.mu.Unlock()

	ctx := context.Background()

	for _, stepID := range stepIDs {
		t.mu.Lock()
		ps, exists := t.pending[stepID]
		if !exists {
			t.mu.Unlock()
			continue
		}
		t.mu.Unlock()

		step, err := t.runStore.GetStep(ctx, stepID)
		if err != nil {
			continue
		}
		// The step record names its run; the registration may not have.
		runID := step.RunID

		if step.TotalJobs == 0 {
			// Jobs haven't been registered yet — wait.
			if time.Since(ps.registeredAt) > 30*time.Minute {
				t.completeStep(ctx, stepID, runID, bstore.StepStatusFailed, "timeout: no jobs registered")
			}
			continue
		}

		// Count completed jobs by checking the job store.
		var doneJobs int
		var allDone bool

		if ps.isExtraction {
			doneJobs, allDone = t.checkJobsByIDs(ctx, step.JobIDs, true)
		} else {
			doneJobs, allDone = t.checkJobsByIDs(ctx, step.JobIDs, false)
		}

		// Update progress.
		if doneJobs != step.DoneJobs {
			_ = t.runStore.UpdateStepJobProgress(ctx, stepID, doneJobs)
			if !allDone {
				notifyRunChange(ctx, t.runStore, t.notifier, AutomationStepProgress, runID)
			}
		}

		if allDone {
			t.completeStep(ctx, stepID, runID, bstore.StepStatusCompleted, "")
		} else if time.Since(ps.registeredAt) > 30*time.Minute {
			t.completeStep(ctx, stepID, runID, bstore.StepStatusFailed, "timeout")
		}
	}
}

func (t *StepCompletionTracker) checkJobsByIDs(ctx context.Context, jobIDs []string, isExtraction bool) (done int, allDone bool) {
	allDone = true
	for _, jid := range jobIDs {
		if isExtraction && t.extractStore != nil {
			j, err := t.extractStore.GetExtractionJob(ctx, jid)
			if err != nil {
				continue
			}
			switch j.Status {
			case jobs.ExtractionStatusCompleted, jobs.ExtractionStatusFailed:
				done++
			default:
				allDone = false
			}
		} else if t.jobStore != nil {
			j, err := t.jobStore.GetJob(ctx, jid)
			if err != nil {
				continue
			}
			switch j.Status {
			case jobs.StatusCompleted, jobs.StatusFailed:
				done++
			default:
				allDone = false
			}
		}
	}
	return done, allDone
}

// completeStep closes a tracked step with the status, credits the run's done
// count, bills the runner time, settles the run when it was the last open
// step, and forgets the step so it is not polled again.
func (t *StepCompletionTracker) completeStep(ctx context.Context, stepID, runID string, status bstore.StepStatus, errMsg string) {
	t.mu.Lock()
	ps := t.pending[stepID]
	delete(t.pending, stepID)
	t.mu.Unlock()

	// Read step to calculate duration before marking complete.
	var durationSec float64
	if step, err := t.runStore.GetStep(ctx, stepID); err == nil && !step.StartedAt.IsZero() {
		durationSec = time.Since(step.StartedAt).Seconds()
	}

	if err := t.runStore.UpdateStepStatus(ctx, stepID, status, errMsg); err != nil {
		slog.Info("step-tracker: failed to update step", "id", stepID, "error", err)
	}
	if err := t.runStore.IncrementDoneCount(ctx, runID); err != nil {
		slog.Info("step-tracker: failed to increment done count for run", "id", runID, "error", err)
	}

	// Record runner usage and deduct billing credits.
	if ps != nil && ps.workspaceID != "" && durationSec > 0 {
		op := ps.actionType
		if op == "" {
			op = "automation"
		}
		// Fail-open by policy: the step has already run and its runner time is
		// already spent, so a meter outage must not turn a completed step into a
		// failed one. Logged and counted rather than discarded, because runner
		// time nobody metered is runner time nobody bills.
		if t.quotaStore != nil {
			if err := t.quotaStore.RecordRunnerUsage(ctx, jobs.RunnerUsageRecord{
				WorkspaceID: ps.workspaceID,
				ProjectID:   ps.projectID,
				Operation:   op,
				DurationSec: durationSec,
				ReferenceID: stepID,
			}); err != nil {
				observe.MeteringDiscarded(ctx, observe.MeterRunnerSeconds, durationSec, err,
					"operation", op, "workspace_id", ps.workspaceID,
					"project_id", ps.projectID, "step_id", stepID, "run_id", runID)
			}
		}
		if t.billingHooks != nil {
			t.billingHooks.DeductContainerTime(ctx, ps.workspaceID, time.Duration(durationSec*float64(time.Second)), stepID)
		}
	}

	_ = t.runStore.AppendLogs(ctx, []bstore.AutomationLog{{
		StepID:  stepID,
		RunID:   runID,
		Level:   "info",
		Message: "Step " + string(status),
	}})
	notifyRunChange(ctx, t.runStore, t.notifier, AutomationStepFinished, runID)

	if settleAutomationRun(ctx, t.runStore, runID) {
		notifyRunChange(ctx, t.runStore, t.notifier, AutomationRunFinished, runID)
	}
}
