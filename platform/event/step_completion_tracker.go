package event

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// StepCompletionTracker monitors async automation steps (auto_translate,
// auto_extract) and updates their status when all spawned jobs complete.
type StepCompletionTracker struct {
	runStore     *bstore.AutomationRunStore
	jobStore     jobs.JobStore
	extractStore jobs.ExtractionJobStore

	mu           sync.Mutex
	pending      map[string]*pendingStep // stepID → state
	pollInterval time.Duration
	done         chan struct{}

	// IsLeader gates polling to the leader instance only. If nil, always polls.
	IsLeader func() bool
}

type pendingStep struct {
	runID        string
	isExtraction bool
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

// TrackStep registers a step for completion tracking.
func (t *StepCompletionTracker) TrackStep(stepID, runID string, isExtraction bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[stepID] = &pendingStep{
		runID:        runID,
		isExtraction: isExtraction,
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
			if t.IsLeader == nil || t.IsLeader() {
				t.checkPending()
			}
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

		if step.TotalJobs == 0 {
			// Jobs haven't been registered yet — wait.
			if time.Since(ps.registeredAt) > 30*time.Minute {
				t.completeStep(ctx, stepID, ps.runID, bstore.StepStatusFailed, "timeout: no jobs registered")
			}
			continue
		}

		// Count completed jobs by checking the job store.
		doneJobs := 0
		allDone := true

		if ps.isExtraction {
			if t.extractStore != nil {
				ejobs, err := t.extractStore.ListByPushID(ctx, stepID) // stepID is not push_id
				_ = ejobs
				_ = err
			}
			// For extraction, check via step's job_ids
			doneJobs, allDone = t.checkJobsByIDs(ctx, step.JobIDs, ps.isExtraction)
		} else {
			doneJobs, allDone = t.checkJobsByIDs(ctx, step.JobIDs, false)
		}

		// Update progress.
		if doneJobs != step.DoneJobs {
			_ = t.runStore.UpdateStepJobProgress(ctx, stepID, doneJobs)
		}

		if allDone {
			t.completeStep(ctx, stepID, ps.runID, bstore.StepStatusCompleted, "")
		} else if time.Since(ps.registeredAt) > 30*time.Minute {
			t.completeStep(ctx, stepID, ps.runID, bstore.StepStatusFailed, "timeout")
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

func (t *StepCompletionTracker) completeStep(ctx context.Context, stepID, runID string, status bstore.StepStatus, errMsg string) {
	if err := t.runStore.UpdateStepStatus(ctx, stepID, status, errMsg); err != nil {
		log.Printf("step-tracker: failed to update step %s: %v", stepID, err)
	}
	if err := t.runStore.IncrementDoneCount(ctx, runID); err != nil {
		log.Printf("step-tracker: failed to increment done count for run %s: %v", runID, err)
	}

	t.runStore.AppendLogs(ctx, []bstore.AutomationLog{{
		StepID:  stepID,
		RunID:   runID,
		Level:   "info",
		Message: "Step " + string(status),
	}})

	// Check if run is complete.
	run, err := t.runStore.GetRun(ctx, runID)
	if err != nil {
		return
	}
	if run.DoneCount >= run.StepCount && run.StepCount > 0 {
		steps, err := t.runStore.ListSteps(ctx, runID)
		if err != nil {
			return
		}
		finalStatus := bstore.RunStatusCompleted
		for _, s := range steps {
			if s.Status == bstore.StepStatusFailed {
				finalStatus = bstore.RunStatusPartial
				break
			}
		}
		_ = t.runStore.UpdateRunStatus(ctx, runID, finalStatus, "")
	}

	t.mu.Lock()
	delete(t.pending, stepID)
	t.mu.Unlock()
}
