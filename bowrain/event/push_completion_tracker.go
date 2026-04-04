package event

import (
	"context"
	"log"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// PushCompletionTracker monitors automation jobs triggered by pushes and
// emits EventPushAutomationsCompleted when all jobs for a push_id finish.
type PushCompletionTracker struct {
	bus          platev.EventBus
	jobStore     jobs.JobStore
	extractStore jobs.ExtractionJobStore
	contentStore platstore.ContentStore
	runStore     *bstore.AutomationRunStore // optional; enables DB-based push discovery for multi-instance
	sub          *platev.Subscription

	mu      sync.Mutex
	pending map[string]*pendingPush

	pollInterval time.Duration
	timeout      time.Duration
	done         chan struct{}
}

type pendingPush struct {
	projectID    string
	items        string
	wsSlug       string
	actor        string
	registeredAt time.Time
}

// PushTrackerOption configures a PushCompletionTracker.
type PushTrackerOption func(*PushCompletionTracker)

// WithPollInterval sets the poll interval (default 5s).
func WithPollInterval(d time.Duration) PushTrackerOption {
	return func(t *PushCompletionTracker) { t.pollInterval = d }
}

// NewPushCompletionTracker creates a tracker that subscribes to push events
// and monitors job completion.
func NewPushCompletionTracker(
	bus platev.EventBus,
	jobStore jobs.JobStore,
	extractStore jobs.ExtractionJobStore,
	contentStore platstore.ContentStore,
	opts ...PushTrackerOption,
) *PushCompletionTracker {
	t := &PushCompletionTracker{
		bus:          bus,
		jobStore:     jobStore,
		extractStore: extractStore,
		contentStore: contentStore,
		pending:      make(map[string]*pendingPush),
		pollInterval: 5 * time.Second,
		timeout:      30 * time.Minute,
		done:         make(chan struct{}),
	}
	for _, opt := range opts {
		opt(t)
	}
	t.sub = bus.SubscribeGroup("push-tracker", t.handlePush)
	go t.pollLoop()
	return t
}

// SetRunStore enables DB-based push discovery for multi-instance deployments.
func (t *PushCompletionTracker) SetRunStore(store *bstore.AutomationRunStore) {
	t.runStore = store
}

// Close stops the tracker and unsubscribes from the event bus.
func (t *PushCompletionTracker) Close() {
	close(t.done)
	if t.sub != nil {
		t.bus.Unsubscribe(t.sub)
	}
}

func (t *PushCompletionTracker) handlePush(ev platev.Event) {
	if ev.Type != platev.EventPushCompleted {
		return // only interested in push events
	}
	pushID := ev.Data["push_id"]
	if pushID == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.pending[pushID]; exists {
		return // already tracking
	}

	t.pending[pushID] = &pendingPush{
		projectID:    ev.ProjectID,
		items:        ev.Data["items"],
		wsSlug:       ev.Data["workspace_slug"],
		actor:        ev.Actor,
		registeredAt: time.Now(),
	}
}

func (t *PushCompletionTracker) pollLoop() {
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			t.ingestFromDB()
			t.checkPending()
		}
	}
}

// ingestFromDB reads pending pushes from the database and registers them
// in the in-memory map. This enables the leader to discover pushes that
// arrived on other server instances (which have their own local event bus).
func (t *PushCompletionTracker) ingestFromDB() {
	if t.runStore == nil {
		return
	}
	ctx := context.Background()
	pushes, err := t.runStore.ListPendingPushes(ctx)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, pp := range pushes {
		if _, exists := t.pending[pp.PushID]; exists {
			continue
		}
		t.pending[pp.PushID] = &pendingPush{
			projectID:    pp.ProjectID,
			items:        pp.Items,
			wsSlug:       pp.WsSlug,
			actor:        pp.Actor,
			registeredAt: pp.CreatedAt,
		}
	}
}

func (t *PushCompletionTracker) checkPending() {
	t.mu.Lock()
	// Copy keys to avoid holding lock during I/O.
	pushIDs := make([]string, 0, len(t.pending))
	for id := range t.pending {
		pushIDs = append(pushIDs, id)
	}
	t.mu.Unlock()

	ctx := context.Background()

	for _, pushID := range pushIDs {
		t.mu.Lock()
		pp, exists := t.pending[pushID]
		if !exists {
			t.mu.Unlock()
			continue
		}
		t.mu.Unlock()

		timedOut := time.Since(pp.registeredAt) > t.timeout
		translationStatus, extractionStatus, allDone := t.checkJobs(ctx, pushID)

		if allDone || timedOut {
			if timedOut && !allDone {
				translationStatus = "timeout"
			}

			t.bus.Publish(platev.Event{
				Type:      platev.EventPushAutomationsCompleted,
				Source:    "push_completion_tracker",
				ProjectID: pp.projectID,
				Actor:     pp.actor,
				Data: map[string]string{
					"push_id":            pushID,
					"items":              pp.items,
					"workspace_slug":     pp.wsSlug,
					"translation_status": translationStatus,
					"extraction_status":  extractionStatus,
				},
			})

			t.mu.Lock()
			delete(t.pending, pushID)
			t.mu.Unlock()

			// Clean up the DB record.
			if t.runStore != nil {
				_ = t.runStore.DeletePendingPush(ctx, pushID)
			}
		}
	}
}

func (t *PushCompletionTracker) checkJobs(ctx context.Context, pushID string) (translationStatus, extractionStatus string, allDone bool) {
	translationStatus = "none"
	extractionStatus = "none"

	// Check translation jobs.
	if t.jobStore != nil {
		tjobs, err := t.jobStore.ListJobsByPushID(ctx, pushID)
		if err != nil {
			log.Printf("push-tracker: failed to list translation jobs for %s: %v", pushID, err)
			return translationStatus, extractionStatus, false
		}
		if len(tjobs) > 0 {
			translationStatus = aggregateJobStatus(tjobs)
			if translationStatus != "all_completed" && translationStatus != "some_failed" {
				return translationStatus, extractionStatus, false
			}
		}
	}

	// Check extraction jobs.
	if t.extractStore != nil {
		ejobs, err := t.extractStore.ListByPushID(ctx, pushID)
		if err != nil {
			log.Printf("push-tracker: failed to list extraction jobs for %s: %v", pushID, err)
			return translationStatus, extractionStatus, false
		}
		if len(ejobs) > 0 {
			extractionStatus = aggregateExtractionStatus(ejobs)
			if extractionStatus != "all_completed" && extractionStatus != "some_failed" {
				return translationStatus, extractionStatus, false
			}
		}
	}

	return translationStatus, extractionStatus, true
}

func aggregateJobStatus(tjobs []*jobs.TranslationJob) string {
	hasFailed := false
	for _, j := range tjobs {
		switch j.Status {
		case jobs.StatusQueued, jobs.StatusProcessing:
			return "in_progress"
		case jobs.StatusFailed:
			hasFailed = true
		}
	}
	if hasFailed {
		return "some_failed"
	}
	return "all_completed"
}

func aggregateExtractionStatus(ejobs []*jobs.ExtractionJob) string {
	hasFailed := false
	for _, j := range ejobs {
		switch j.Status {
		case jobs.ExtractionStatusQueued, jobs.ExtractionStatusProcessing:
			return "in_progress"
		case jobs.ExtractionStatusFailed:
			hasFailed = true
		}
	}
	if hasFailed {
		return "some_failed"
	}
	return "all_completed"
}
