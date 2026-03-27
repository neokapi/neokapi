package event

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/jobs"
	platev "github.com/neokapi/neokapi/platform/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubJobStore is a minimal JobStore for testing.
type stubJobStore struct {
	jobs map[string][]*jobs.TranslationJob
}

func (s *stubJobStore) CreateJob(ctx context.Context, job *jobs.TranslationJob) error { return nil }
func (s *stubJobStore) GetJob(ctx context.Context, id string) (*jobs.TranslationJob, error) {
	return nil, nil
}
func (s *stubJobStore) ListJobs(ctx context.Context, workspaceSlug string, limit int) ([]*jobs.TranslationJob, error) {
	return nil, nil
}
func (s *stubJobStore) UpdateJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks int) error {
	return nil
}
func (s *stubJobStore) UpdateJobStatus(ctx context.Context, id string, status jobs.JobStatus, errMsg string) error {
	return nil
}
func (s *stubJobStore) DeleteJob(ctx context.Context, id string) error { return nil }
func (s *stubJobStore) ListJobsByPushID(ctx context.Context, pushID string) ([]*jobs.TranslationJob, error) {
	return s.jobs[pushID], nil
}

// stubExtractionStore is a minimal ExtractionJobStore for testing.
type stubExtractionStore struct {
	jobs map[string][]*jobs.ExtractionJob
}

func (s *stubExtractionStore) CreateExtractionJob(ctx context.Context, job *jobs.ExtractionJob) error {
	return nil
}
func (s *stubExtractionStore) GetExtractionJob(ctx context.Context, id string) (*jobs.ExtractionJob, error) {
	return nil, nil
}
func (s *stubExtractionStore) UpdateExtractionJobStatus(ctx context.Context, id string, status jobs.ExtractionJobStatus, errMsg string) error {
	return nil
}
func (s *stubExtractionStore) UpdateExtractionJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks, itemsCreated int) error {
	return nil
}
func (s *stubExtractionStore) ListByPushID(ctx context.Context, pushID string) ([]*jobs.ExtractionJob, error) {
	return s.jobs[pushID], nil
}

func TestPushCompletionTracker_AllJobsCompleted(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	jstore := &stubJobStore{jobs: map[string][]*jobs.TranslationJob{
		"push-1": {
			{ID: "j1", Status: jobs.StatusCompleted},
			{ID: "j2", Status: jobs.StatusCompleted},
		},
	}}
	estore := &stubExtractionStore{jobs: map[string][]*jobs.ExtractionJob{
		"push-1": {
			{ID: "e1", Status: jobs.ExtractionStatusCompleted},
		},
	}}

	tracker := NewPushCompletionTracker(bus, jstore, estore, nil)
	tracker.pollInterval = 50 * time.Millisecond
	defer tracker.Close()

	// Capture emitted events.
	var received []platev.Event
	bus.Subscribe(platev.EventPushAutomationsCompleted, func(ev platev.Event) {
		received = append(received, ev)
	})

	// Simulate a push event.
	bus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
		Actor:     "user-1",
		Data: map[string]string{
			"push_id":        "push-1",
			"items":          "en.json",
			"workspace_slug": "test-ws",
		},
	})

	// Wait for the tracker to poll and emit.
	require.Eventually(t, func() bool {
		return len(received) > 0
	}, 2*time.Second, 50*time.Millisecond)

	assert.Equal(t, platev.EventPushAutomationsCompleted, received[0].Type)
	assert.Equal(t, "proj-1", received[0].ProjectID)
	assert.Equal(t, "all_completed", received[0].Data["translation_status"])
	assert.Equal(t, "all_completed", received[0].Data["extraction_status"])
	assert.Equal(t, "push-1", received[0].Data["push_id"])
}

func TestPushCompletionTracker_ZeroJobs(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	jstore := &stubJobStore{jobs: map[string][]*jobs.TranslationJob{}}
	estore := &stubExtractionStore{jobs: map[string][]*jobs.ExtractionJob{}}

	tracker := NewPushCompletionTracker(bus, jstore, estore, nil)
	tracker.pollInterval = 50 * time.Millisecond
	defer tracker.Close()

	var received []platev.Event
	bus.Subscribe(platev.EventPushAutomationsCompleted, func(ev platev.Event) {
		received = append(received, ev)
	})

	bus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
		Data: map[string]string{
			"push_id":        "push-2",
			"items":          "en.json",
			"workspace_slug": "test-ws",
		},
	})

	require.Eventually(t, func() bool {
		return len(received) > 0
	}, 2*time.Second, 50*time.Millisecond)

	assert.Equal(t, "none", received[0].Data["translation_status"])
	assert.Equal(t, "none", received[0].Data["extraction_status"])
}

func TestPushCompletionTracker_InProgressWaits(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	tjobs := []*jobs.TranslationJob{
		{ID: "j1", Status: jobs.StatusProcessing},
	}
	jstore := &stubJobStore{jobs: map[string][]*jobs.TranslationJob{
		"push-3": tjobs,
	}}
	estore := &stubExtractionStore{jobs: map[string][]*jobs.ExtractionJob{}}

	tracker := NewPushCompletionTracker(bus, jstore, estore, nil)
	tracker.pollInterval = 50 * time.Millisecond
	defer tracker.Close()

	var received []platev.Event
	bus.Subscribe(platev.EventPushAutomationsCompleted, func(ev platev.Event) {
		received = append(received, ev)
	})

	bus.Publish(platev.Event{
		Type: platev.EventPushCompleted,
		Data: map[string]string{
			"push_id":        "push-3",
			"items":          "en.json",
			"workspace_slug": "test-ws",
		},
	})

	// Should NOT emit while jobs are in progress.
	time.Sleep(200 * time.Millisecond)
	assert.Empty(t, received)

	// Complete the job.
	tjobs[0].Status = jobs.StatusCompleted

	// Now it should emit.
	require.Eventually(t, func() bool {
		return len(received) > 0
	}, 2*time.Second, 50*time.Millisecond)

	assert.Equal(t, "all_completed", received[0].Data["translation_status"])
}
