package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestNotificationStore(t *testing.T) *NotificationStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return NewNotificationStore(s.DB())
}

func TestNotificationStore_CreateAndList(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := context.Background()

	require.NoError(t, ns.Create(ctx, &Notification{
		UserID:    "user-1",
		Type:      NotificationReviewAssigned,
		Title:     "Review assigned",
		Body:      "You have 5 new items to review",
		ProjectID: "proj-1",
	}))
	require.NoError(t, ns.Create(ctx, &Notification{
		UserID: "user-1",
		Type:   NotificationExtractionDone,
		Title:  "Extraction complete",
	}))
	require.NoError(t, ns.Create(ctx, &Notification{
		UserID: "user-2",
		Type:   NotificationGeneral,
		Title:  "Hello",
	}))

	// List all for user-1.
	notifs, err := ns.List(ctx, "user-1", 50, false)
	require.NoError(t, err)
	assert.Len(t, notifs, 2)
	// Verify both titles present.
	titles := []string{notifs[0].Title, notifs[1].Title}
	assert.Contains(t, titles, "Review assigned")
	assert.Contains(t, titles, "Extraction complete")

	// List for user-2.
	notifs, err = ns.List(ctx, "user-2", 50, false)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
}

func TestNotificationStore_UnreadCount(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := context.Background()

	require.NoError(t, ns.Create(ctx, &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "A",
	}))
	require.NoError(t, ns.Create(ctx, &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "B",
	}))

	count, err := ns.UnreadCount(ctx, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestNotificationStore_MarkRead(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := context.Background()

	n := &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "Test",
	}
	require.NoError(t, ns.Create(ctx, n))

	require.NoError(t, ns.MarkRead(ctx, n.ID, "user-1"))

	count, err := ns.UnreadCount(ctx, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Unread only returns nothing.
	notifs, err := ns.List(ctx, "user-1", 50, true)
	require.NoError(t, err)
	assert.Len(t, notifs, 0)

	// All returns 1.
	notifs, err = ns.List(ctx, "user-1", 50, false)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.True(t, notifs[0].Read)
}

func TestNotificationStore_MarkAllRead(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		require.NoError(t, ns.Create(ctx, &Notification{
			UserID: "user-1", Type: NotificationGeneral, Title: "N",
		}))
	}

	require.NoError(t, ns.MarkAllRead(ctx, "user-1"))

	count, err := ns.UnreadCount(ctx, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestNotificationStore_Delete(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := context.Background()

	n := &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "Delete me",
	}
	require.NoError(t, ns.Create(ctx, n))

	require.NoError(t, ns.Delete(ctx, n.ID, "user-1"))

	notifs, err := ns.List(ctx, "user-1", 50, false)
	require.NoError(t, err)
	assert.Len(t, notifs, 0)
}
