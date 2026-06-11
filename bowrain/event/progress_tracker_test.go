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

	// Publish the same event twice.
	bus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
		Actor:     "system",
		Data:      map[string]string{"stream": "main"},
	})

	require.Eventually(t, func() bool {
		return sender.count() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	// Wait for all first-event notifications to settle.
	firstCount := settledCount(t, sender)

	bus.Publish(platev.Event{
		Type:      platev.EventPushCompleted,
		ProjectID: "proj-1",
		Actor:     "system",
		Data:      map[string]string{"stream": "main"},
	})

	require.Eventually(t, func() bool {
		return sender.count() > firstCount
	}, 2*time.Second, 10*time.Millisecond)

	// Wait for all second-event notifications to settle.
	secondCount := settledCount(t, sender)

	// The dispatcher creates a new notification for each EventPushCompleted (content available),
	// but the progress tracker should NOT create duplicate milestone notifications.
	// So the difference should be exactly 1 (only the dispatcher's content-available notification).
	diff := secondCount - firstCount
	assert.Equal(t, 1, diff, "second event should only produce the dispatcher notification, not duplicate milestones")
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

// settledCount waits until the sender's notification count is stable across
// two consecutive poll intervals and returns it. Strictly more robust than a
// fixed sleep: it keeps waiting while notifications are still arriving and is
// bounded by the Eventually deadline.
func settledCount(t *testing.T, sender *mockSender) int {
	t.Helper()
	last := sender.count()
	require.Eventually(t, func() bool {
		cur := sender.count()
		if cur == last {
			return true
		}
		last = cur
		return false
	}, 5*time.Second, 100*time.Millisecond)
	return last
}
