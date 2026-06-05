package store

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestNotificationStore(t *testing.T) *NotificationStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	_, err := NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return NewNotificationStore(db.DB)
}

func TestNotificationStore_CreateAndList(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()

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
	ctx := t.Context()

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
	ctx := t.Context()

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
	assert.Empty(t, notifs)

	// All returns 1.
	notifs, err = ns.List(ctx, "user-1", 50, false)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.True(t, notifs[0].Read)
}

func TestNotificationStore_MarkAllRead(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()

	for range 5 {
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
	ctx := t.Context()

	n := &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "Delete me",
	}
	require.NoError(t, ns.Create(ctx, n))

	require.NoError(t, ns.Delete(ctx, n.ID, "user-1"))

	notifs, err := ns.List(ctx, "user-1", 50, false)
	require.NoError(t, err)
	assert.Empty(t, notifs)
}

func TestNotificationStore_MarkReadByGroupKey(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()

	// Create three notifications with the same group key.
	for _, title := range []string{"A", "B", "C"} {
		require.NoError(t, ns.Create(ctx, &Notification{
			UserID:   "user-1",
			Type:     NotificationGeneral,
			Title:    title,
			GroupKey: "grp-1",
		}))
	}
	// Create one with a different group key.
	require.NoError(t, ns.Create(ctx, &Notification{
		UserID:   "user-1",
		Type:     NotificationGeneral,
		Title:    "D",
		GroupKey: "grp-2",
	}))

	require.NoError(t, ns.MarkReadByGroupKey(ctx, "grp-1"))

	// All grp-1 notifications should be read.
	notifs, err := ns.List(ctx, "user-1", 50, true)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Equal(t, "D", notifs[0].Title)

	// Total should still be 4.
	notifs, err = ns.List(ctx, "user-1", 50, false)
	require.NoError(t, err)
	assert.Len(t, notifs, 4)
}

func TestNotificationStore_MarkReadByGroupKey_EmptyKey(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()

	require.NoError(t, ns.Create(ctx, &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "A",
	}))

	// Empty key should be a no-op (no error, nothing marked).
	require.NoError(t, ns.MarkReadByGroupKey(ctx, ""))

	count, err := ns.UnreadCount(ctx, "user-1")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestNotificationStore_ListUnreadSince(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()

	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)
	t2 := t0.Add(2 * time.Hour)
	t3 := t0.Add(3 * time.Hour)

	// Create notifications at different times.
	require.NoError(t, ns.Create(ctx, &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "Old", CreatedAt: t1,
	}))
	require.NoError(t, ns.Create(ctx, &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "Mid", CreatedAt: t2,
	}))
	require.NoError(t, ns.Create(ctx, &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "New", CreatedAt: t3,
	}))

	// Since t2 should return only the notification created after t2.
	notifs, err := ns.ListUnreadSince(ctx, "user-1", t2)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Equal(t, "New", notifs[0].Title)

	// Since t0 should return all three.
	notifs, err = ns.ListUnreadSince(ctx, "user-1", t0)
	require.NoError(t, err)
	assert.Len(t, notifs, 3)
}

func TestNotificationStore_ListUnreadSince_SkipsRead(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()

	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create two notifications.
	n1 := &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "Read",
		CreatedAt: t0.Add(1 * time.Hour),
	}
	require.NoError(t, ns.Create(ctx, n1))
	require.NoError(t, ns.Create(ctx, &Notification{
		UserID: "user-1", Type: NotificationGeneral, Title: "Unread",
		CreatedAt: t0.Add(2 * time.Hour),
	}))

	// Mark the first as read.
	require.NoError(t, ns.MarkRead(ctx, n1.ID, "user-1"))

	// ListUnreadSince should only return the unread one.
	notifs, err := ns.ListUnreadSince(ctx, "user-1", t0)
	require.NoError(t, err)
	assert.Len(t, notifs, 1)
	assert.Equal(t, "Unread", notifs[0].Title)
}
