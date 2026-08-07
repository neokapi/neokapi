package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNotificationStore_OneEventOnePersonOneRow pins the verdict Create
// returns. It is what tells the dispatcher whether to push and mail: a person
// hears about one event once, however many times the queue delivers it.
func TestNotificationStore_OneEventOnePersonOneRow(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()

	first, err := ns.Create(ctx, &Notification{
		UserID: "u1", Type: NotificationGateFailed, Title: "Quality gate failed",
		SourceEventID: "evt-1",
	})
	require.NoError(t, err)
	assert.True(t, first, "the first telling is new")

	for range 3 {
		again, err := ns.Create(ctx, &Notification{
			UserID: "u1", Type: NotificationGateFailed, Title: "Quality gate failed",
			SourceEventID: "evt-1",
		})
		require.NoError(t, err)
		assert.False(t, again, "a redelivery is not")
	}

	list, err := ns.List(ctx, "u1", 50, false)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

// TestNotificationStore_EventReachesEveryTarget keeps the key per-user: one
// event notifies every member of the project, and the guard must not collapse
// them into one.
func TestNotificationStore_EventReachesEveryTarget(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()

	for _, user := range []string{"u1", "u2", "u3"} {
		created, err := ns.Create(ctx, &Notification{
			UserID: user, Type: NotificationContentAvailable, Title: "New content available",
			SourceEventID: "evt-push",
		})
		require.NoError(t, err)
		assert.True(t, created, "%s is a different person", user)
	}

	for _, user := range []string{"u1", "u2", "u3"} {
		list, err := ns.List(ctx, user, 50, false)
		require.NoError(t, err)
		assert.Len(t, list, 1, "%s", user)
	}
}

// TestNotificationStore_UnsourcedNotificationsStayIndependent keeps the guard
// off the notifications raised directly. A mention, a deadline and a task
// assignment have no event behind them, and two of them are two notifications.
func TestNotificationStore_UnsourcedNotificationsStayIndependent(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()

	for range 3 {
		created, err := ns.Create(ctx, &Notification{
			UserID: "u1", Type: NotificationMention, Title: "alice mentioned you",
		})
		require.NoError(t, err)
		assert.True(t, created)
	}

	list, err := ns.List(ctx, "u1", 50, false)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}
