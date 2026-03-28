package event

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/jobs"
	platev "github.com/neokapi/neokapi/platform/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubJobStore is a minimal JobStore for testing.
type stubJobStore struct {
	mu   sync.Mutex
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
func (s *stubJobStore) ClaimJob(ctx context.Context, id string) (bool, error) {
	return true, nil
}
func (s *stubJobStore) ListJobsByPushID(ctx context.Context, pushID string) ([]*jobs.TranslationJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.jobs[pushID]
	// Return a copy so the caller doesn't race on status fields.
	out := make([]*jobs.TranslationJob, len(src))
	for i, j := range src {
		cp := *j
		out[i] = &cp
	}
	return out, nil
}

func (s *stubJobStore) setStatus(pushID, jobID string, status jobs.JobStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs[pushID] {
		if j.ID == jobID {
			j.Status = status
		}
	}
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
func (s *stubExtractionStore) ClaimExtractionJob(ctx context.Context, id string) (bool, error) {
	return true, nil
}
func (s *stubExtractionStore) ListByPushID(ctx context.Context, pushID string) ([]*jobs.ExtractionJob, error) {
	return s.jobs[pushID], nil
}

// eventCollector collects events in a thread-safe way.
type eventCollector struct {
	mu     sync.Mutex
	events []platev.Event
}

func (c *eventCollector) handler(ev platev.Event) {
	c.mu.Lock()
	c.events = append(c.events, ev)
	c.mu.Unlock()
}

func (c *eventCollector) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func (c *eventCollector) get(i int) platev.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.events[i]
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

	tracker := NewPushCompletionTracker(bus, jstore, estore, nil, WithPollInterval(50*time.Millisecond))
	defer tracker.Close()

	collector := &eventCollector{}
	bus.Subscribe(platev.EventPushAutomationsCompleted, collector.handler)

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

	require.Eventually(t, func() bool {
		return collector.len() > 0
	}, 2*time.Second, 50*time.Millisecond)

	ev := collector.get(0)
	assert.Equal(t, platev.EventPushAutomationsCompleted, ev.Type)
	assert.Equal(t, "proj-1", ev.ProjectID)
	assert.Equal(t, "all_completed", ev.Data["translation_status"])
	assert.Equal(t, "all_completed", ev.Data["extraction_status"])
	assert.Equal(t, "push-1", ev.Data["push_id"])
}

func TestPushCompletionTracker_ZeroJobs(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	jstore := &stubJobStore{jobs: map[string][]*jobs.TranslationJob{}}
	estore := &stubExtractionStore{jobs: map[string][]*jobs.ExtractionJob{}}

	tracker := NewPushCompletionTracker(bus, jstore, estore, nil, WithPollInterval(50*time.Millisecond))
	defer tracker.Close()

	collector := &eventCollector{}
	bus.Subscribe(platev.EventPushAutomationsCompleted, collector.handler)

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
		return collector.len() > 0
	}, 2*time.Second, 50*time.Millisecond)

	ev := collector.get(0)
	assert.Equal(t, "none", ev.Data["translation_status"])
	assert.Equal(t, "none", ev.Data["extraction_status"])
}

func TestPushCompletionTracker_InProgressWaits(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	jstore := &stubJobStore{jobs: map[string][]*jobs.TranslationJob{
		"push-3": {
			{ID: "j1", Status: jobs.StatusProcessing},
		},
	}}
	estore := &stubExtractionStore{jobs: map[string][]*jobs.ExtractionJob{}}

	tracker := NewPushCompletionTracker(bus, jstore, estore, nil, WithPollInterval(50*time.Millisecond))
	defer tracker.Close()

	collector := &eventCollector{}
	bus.Subscribe(platev.EventPushAutomationsCompleted, collector.handler)

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
	assert.Equal(t, 0, collector.len())

	// Complete the job via thread-safe setter.
	jstore.setStatus("push-3", "j1", jobs.StatusCompleted)

	// Now it should emit.
	require.Eventually(t, func() bool {
		return collector.len() > 0
	}, 2*time.Second, 50*time.Millisecond)

	ev := collector.get(0)
	assert.Equal(t, "all_completed", ev.Data["translation_status"])
}
