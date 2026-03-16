package event

import (
	"context"
	"sync"
	"testing"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	platev "github.com/neokapi/neokapi/platform/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSender struct {
	mu            sync.Mutex
	notifications []*bstore.Notification
}

func (m *mockSender) NotifyUser(userID string, n *bstore.Notification) {
	m.mu.Lock()
	m.notifications = append(m.notifications, n)
	m.mu.Unlock()
}

func (m *mockSender) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.notifications)
}

func newTestNotifStore(t *testing.T) *bstore.NotificationStore {
	t.Helper()
	s, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return bstore.NewNotificationStore(s.DB())
}

func TestNotificationDispatcher_FlowFailed(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	notifStore := newTestNotifStore(t)
	sender := &mockSender{}

	targetFn := func(ctx context.Context, projectID, excludeActorID string) ([]string, error) {
		return []string{"user-1", "user-2"}, nil
	}

	d := NewNotificationDispatcher(bus, notifStore, nil, sender, targetFn)
	defer d.Close()

	bus.Publish(platev.Event{
		Type:      platev.EventFlowFailed,
		ProjectID: "proj-1",
		Actor:     "system",
		Data:      map[string]string{"actor_name": "System", "workspace_slug": "ws-1"},
	})

	time.Sleep(100 * time.Millisecond)

	// Both users should be notified.
	assert.Equal(t, 2, sender.count())

	// Verify persisted.
	notifs, err := notifStore.List(context.Background(), "user-1", 10, false)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, bstore.NotificationFlowFailed, notifs[0].Type)
}

func TestNotificationDispatcher_ExcludesActor(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	notifStore := newTestNotifStore(t)
	sender := &mockSender{}

	targetFn := func(ctx context.Context, projectID, excludeActorID string) ([]string, error) {
		// Simulate excluding the actor.
		users := []string{"user-1", "user-2", "user-3"}
		var result []string
		for _, u := range users {
			if u != excludeActorID {
				result = append(result, u)
			}
		}
		return result, nil
	}

	d := NewNotificationDispatcher(bus, notifStore, nil, sender, targetFn)
	defer d.Close()

	bus.Publish(platev.Event{
		Type:      platev.EventQualityGateFail,
		ProjectID: "proj-1",
		Actor:     "user-1",
		Data:      map[string]string{"workspace_slug": "ws-1"},
	})

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 2, sender.count()) // user-2, user-3 (not user-1)
}

func TestNotificationDispatcher_SkipsUnmappedEvents(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	notifStore := newTestNotifStore(t)
	sender := &mockSender{}

	targetFn := func(ctx context.Context, projectID, excludeActorID string) ([]string, error) {
		return []string{"user-1"}, nil
	}

	d := NewNotificationDispatcher(bus, notifStore, nil, sender, targetFn)
	defer d.Close()

	bus.Publish(platev.Event{
		Type:      platev.EventProjectCreated, // Not mapped to notification.
		ProjectID: "proj-1",
		Data:      map[string]string{"workspace_slug": "ws-1"},
	})

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 0, sender.count())
}

func TestNotificationDispatcher_PreferencesOptOut(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	s, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer s.Close()

	notifStore := bstore.NewNotificationStore(s.DB())
	prefStore := bstore.NewPreferenceStore(s.DB())
	sender := &mockSender{}

	ctx := context.Background()

	// User-1 opts out of automation notifications.
	require.NoError(t, prefStore.Upsert(ctx, bstore.NotificationPreference{
		UserID:      "user-1",
		WorkspaceID: "ws-1",
		Category:    bstore.CategoryAutomation,
		Web:         false,
	}))

	targetFn := func(ctx context.Context, projectID, excludeActorID string) ([]string, error) {
		return []string{"user-1", "user-2"}, nil
	}

	d := NewNotificationDispatcher(bus, notifStore, prefStore, sender, targetFn)
	defer d.Close()

	bus.Publish(platev.Event{
		Type:      platev.EventFlowFailed,
		ProjectID: "proj-1",
		Data:      map[string]string{"workspace_slug": "ws-1"},
	})

	time.Sleep(100 * time.Millisecond)

	// Only user-2 should be notified (user-1 opted out).
	assert.Equal(t, 1, sender.count())
}

func TestNotificationDispatcher_DispatchTaskNotification(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	notifStore := newTestNotifStore(t)
	sender := &mockSender{}

	d := NewNotificationDispatcher(bus, notifStore, nil, sender, nil)
	defer d.Close()

	task := &bstore.Task{
		ID:         "task-1",
		ProjectID:  "proj-1",
		AssigneeID: "user-2",
		CreatedBy:  "user-1",
		Title:      "Review translations",
		Priority:   bstore.TaskPriorityHigh,
	}

	ctx := context.Background()
	d.DispatchTaskNotification(ctx, task, bstore.NotificationTaskAssigned, "Task assigned", "You have a new task")

	assert.Equal(t, 1, sender.count())

	notifs, err := notifStore.List(ctx, "user-2", 10, false)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, bstore.NotificationTaskAssigned, notifs[0].Type)
	assert.Equal(t, "task-1", notifs[0].TaskID)
}

func TestNotificationDispatcher_TaskNotificationSkipsNoAssignee(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	notifStore := newTestNotifStore(t)
	sender := &mockSender{}

	d := NewNotificationDispatcher(bus, notifStore, nil, sender, nil)
	defer d.Close()

	task := &bstore.Task{
		ID:        "task-1",
		ProjectID: "proj-1",
		CreatedBy: "user-1",
		Title:     "Unassigned task",
	}

	d.DispatchTaskNotification(context.Background(), task, bstore.NotificationTaskAssigned, "Task assigned", "Body")

	assert.Equal(t, 0, sender.count())
}
