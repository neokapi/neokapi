package event

import (
	"context"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockContentStore is a minimal ContentStore implementation for progress tracker tests.
// Only GetProject and GetBlockStats are implemented; all other methods panic.
type mockContentStore struct {
	store.ContentStore
	project    *store.Project
	blockStats []store.BlockStatRow
}

func (m *mockContentStore) GetProject(_ context.Context, id string) (*store.Project, error) {
	if m.project != nil && m.project.ID == id {
		return m.project, nil
	}
	return nil, context.DeadlineExceeded
}

func (m *mockContentStore) GetBlockStats(_ context.Context, projectID, stream string) ([]store.BlockStatRow, error) {
	return m.blockStats, nil
}

func newProgressTestSetup(t *testing.T, cs store.ContentStore) (*ChannelEventBus, *mockSender, *ProgressTracker) {
	t.Helper()

	bus := NewChannelEventBus()
	t.Cleanup(func() { bus.Close() })

	db := pgtest.NewTestDB(t)
	_, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)

	notifStore := bstore.NewNotificationStore(db.DB)
	sender := &mockSender{}

	targetFn := func(ctx context.Context, projectID, excludeActorID string) ([]string, error) {
		return []string{"user-1"}, nil
	}

	dispatcher := NewNotificationDispatcher(bus, notifStore, nil, sender, targetFn)
	t.Cleanup(func() { dispatcher.Close() })

	pt := NewProgressTracker(cs, dispatcher, bus)
	t.Cleanup(func() { pt.Close() })

	return bus, sender, pt
}

func TestProgressTracker_DetectsMilestone(t *testing.T) {
	proj := &store.Project{
		ID:              "proj-1",
		Name:            "TestProject",
		TargetLanguages: []model.LocaleID{"fr-FR"},
		DefaultStream:   "main",
	}

	// All 3 translatable blocks have fr-FR translated → 100%.
	cs := &mockContentStore{
		project: proj,
		blockStats: []store.BlockStatRow{
			{ItemName: "file1", Translatable: true, SourceWords: 10, TargetLocales: []string{"fr-FR"}},
			{ItemName: "file1", Translatable: true, SourceWords: 5, TargetLocales: []string{"fr-FR"}},
			{ItemName: "file1", Translatable: true, SourceWords: 8, TargetLocales: []string{"fr-FR"}},
		},
	}

	bus, sender, _ := newProgressTestSetup(t, cs)

	bus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
		Actor:     "system",
		Data:      map[string]string{"stream": "main"},
	})

	// At 100%, milestones 25, 50, 75, 100 should all fire → 4 notifications
	// (dispatcher also creates its own notification for EventPushCompleted, so +1 = 5).
	require.Eventually(t, func() bool {
		return sender.count() >= 4
	}, 2*time.Second, 10*time.Millisecond)
}

func TestProgressTracker_NoDuplicate(t *testing.T) {
	proj := &store.Project{
		ID:              "proj-1",
		Name:            "TestProject",
		TargetLanguages: []model.LocaleID{"fr-FR"},
		DefaultStream:   "main",
	}

	cs := &mockContentStore{
		project: proj,
		blockStats: []store.BlockStatRow{
			{ItemName: "file1", Translatable: true, SourceWords: 10, TargetLocales: []string{"fr-FR"}},
		},
	}

	bus, sender, _ := newProgressTestSetup(t, cs)

	publish := func() {
		bus.Publish(platev.Event{
			Type:      platev.EventPushCompleted,
			ProjectID: "proj-1",
			Actor:     "system",
			Data:      map[string]string{"stream": "main"},
		})
	}

	// First push at 100%: the complete, KNOWN set is 4 milestone notifications
	// (25/50/75/100) from the tracker plus the dispatcher's content-available.
	// Waiting for that exact set — instead of a count-settling heuristic —
	// keeps the boundary between the two events deterministic on a slow,
	// loaded runner, where gaps between notifications can exceed any settle
	// interval (each one is a real Postgres write under -race).
	publish()
	require.Eventually(t, func() bool {
		return countOfType(sender, bstore.NotificationProgressMilestone) == 4 &&
			countOfType(sender, bstore.NotificationContentAvailable) == 1
	}, 10*time.Second, 10*time.Millisecond)

	// Second, identical push: the dispatcher notifies again (new content), but
	// the tracker must NOT re-notify milestones it has already seen.
	publish()
	require.Eventually(t, func() bool {
		return countOfType(sender, bstore.NotificationContentAvailable) == 2
	}, 10*time.Second, 10*time.Millisecond)

	// The tracker handles the event concurrently with the dispatcher, so give
	// a would-be duplicate milestone a window to surface before asserting.
	assert.Never(t, func() bool {
		return countOfType(sender, bstore.NotificationProgressMilestone) > 4
	}, time.Second, 50*time.Millisecond)
}

// countOfType counts the sender's notifications of one type. Type-scoped
// counts let the progress tests tell tracker milestones apart from the
// dispatcher's own notifications without relying on arrival timing.
func countOfType(sender *mockSender, nt bstore.NotificationType) int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	n := 0
	for _, x := range sender.notifications {
		if x.Type == nt {
			n++
		}
	}
	return n
}

func TestProgressTracker_IgnoresNonBatchEvents(t *testing.T) {
	proj := &store.Project{
		ID:              "proj-1",
		Name:            "TestProject",
		TargetLanguages: []model.LocaleID{"fr-FR"},
		DefaultStream:   "main",
	}

	cs := &mockContentStore{
		project: proj,
		blockStats: []store.BlockStatRow{
			{ItemName: "file1", Translatable: true, SourceWords: 10, TargetLocales: []string{"fr-FR"}},
		},
	}

	bus, sender, _ := newProgressTestSetup(t, cs)

	bus.Publish(platev.Event{
		Type:      platev.EventBlockUpdated,
		ProjectID: "proj-1",
		Actor:     "system",
		Data:      map[string]string{"stream": "main"},
	})

	// EventBlockUpdated is not mapped by the notification dispatcher either,
	// so no notifications at all. Give bus time to deliver.
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, sender.count(), "non-batch events should not trigger progress checks or notifications")
}

func TestProgressTracker_IgnoresNoProjectID(t *testing.T) {
	proj := &store.Project{
		ID:              "proj-1",
		Name:            "TestProject",
		TargetLanguages: []model.LocaleID{"fr-FR"},
		DefaultStream:   "main",
	}

	cs := &mockContentStore{
		project: proj,
		blockStats: []store.BlockStatRow{
			{ItemName: "file1", Translatable: true, SourceWords: 10, TargetLocales: []string{"fr-FR"}},
		},
	}

	bus, sender, _ := newProgressTestSetup(t, cs)

	// Publish a batch event without ProjectID — should be ignored by progress tracker.
	bus.Publish(platev.Event{
		Type:  platev.EventPushCompleted,
		Actor: "system",
		Data:  map[string]string{"stream": "main"},
	})

	// The dispatcher also won't resolve targets without a ProjectID (targetFn is keyed on it),
	// so no notifications should be created. Give bus time to deliver.
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, sender.count(), "events without ProjectID should be ignored")
}
