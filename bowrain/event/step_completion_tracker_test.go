package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepTrackingInfo_Defaults(t *testing.T) {
	info := StepTrackingInfo{
		StepID: "step-1",
		RunID:  "run-1",
	}
	assert.Equal(t, "step-1", info.StepID)
	assert.Equal(t, "run-1", info.RunID)
	assert.False(t, info.Extraction)
	assert.Empty(t, info.WorkspaceID)
	assert.Empty(t, info.ProjectID)
	assert.Empty(t, info.ActionType)
}

func TestStepTrackingInfo_FullContext(t *testing.T) {
	info := StepTrackingInfo{
		StepID:      "step-42",
		RunID:       "run-7",
		Extraction:  true,
		WorkspaceID: "ws-123",
		ProjectID:   "proj-456",
		ActionType:  "auto_extract",
	}
	assert.Equal(t, "step-42", info.StepID)
	assert.True(t, info.Extraction)
	assert.Equal(t, "ws-123", info.WorkspaceID)
	assert.Equal(t, "auto_extract", info.ActionType)
}

func TestTrackStep_BackwardCompatibility(t *testing.T) {
	// TrackStep (old API) should still work, creating a pendingStep
	// with empty billing context.
	tracker := &StepCompletionTracker{
		pending: make(map[string]*pendingStep),
		done:    make(chan struct{}),
	}

	tracker.TrackStep("step-1", "run-1", false)

	tracker.mu.Lock()
	ps, ok := tracker.pending["step-1"]
	tracker.mu.Unlock()

	assert.True(t, ok)
	assert.Equal(t, "run-1", ps.runID)
	assert.False(t, ps.isExtraction)
	assert.Empty(t, ps.workspaceID)
	assert.Empty(t, ps.actionType)
}

func TestTrackStepWithInfo_StoresBillingContext(t *testing.T) {
	tracker := &StepCompletionTracker{
		pending: make(map[string]*pendingStep),
		done:    make(chan struct{}),
	}

	tracker.TrackStepWithInfo(StepTrackingInfo{
		StepID:      "step-99",
		RunID:       "run-5",
		Extraction:  false,
		WorkspaceID: "ws-abc",
		ProjectID:   "proj-def",
		ActionType:  "auto_translate",
	})

	tracker.mu.Lock()
	ps, ok := tracker.pending["step-99"]
	tracker.mu.Unlock()

	assert.True(t, ok)
	assert.Equal(t, "run-5", ps.runID)
	assert.Equal(t, "ws-abc", ps.workspaceID)
	assert.Equal(t, "proj-def", ps.projectID)
	assert.Equal(t, "auto_translate", ps.actionType)
}

func TestSetBillingHooks_NilSafe(t *testing.T) {
	tracker := &StepCompletionTracker{
		pending: make(map[string]*pendingStep),
		done:    make(chan struct{}),
	}

	// Should not panic.
	tracker.SetBillingHooks(nil)
	assert.Nil(t, tracker.billingHooks)
}

func TestSetQuotaStore_NilSafe(t *testing.T) {
	tracker := &StepCompletionTracker{
		pending: make(map[string]*pendingStep),
		done:    make(chan struct{}),
	}

	// Should not panic.
	tracker.SetQuotaStore(nil)
	assert.Nil(t, tracker.quotaStore)
}
