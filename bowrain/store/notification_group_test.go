package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNotificationStore_Groups pins the queries the grouped job-failure summons
// reads on PostgreSQL: a group opened by source_event_id, found by prefix
// within a window, counted for the email ceiling, rewritten as it grows, and
// sized by its members exactly once each.
func TestNotificationStore_Groups(t *testing.T) {
	ns := newTestNotificationStore(t)
	ctx := t.Context()
	now := time.Now().UTC()

	open := func(user, key string) bool {
		t.Helper()
		created, err := ns.Create(ctx, &Notification{
			UserID: user, Type: NotificationFlowFailed, Title: "A translation job did not finish",
			GroupKey: key, SourceEventID: key,
		})
		require.NoError(t, err)
		return created
	}

	// The group key is the claim: the first insert opens it, a second is refused.
	assert.True(t, open("u1", "job-failures:ws_1:owners:abc:100"))
	assert.False(t, open("u1", "job-failures:ws_1:owners:abc:100"))
	assert.True(t, open("u1", "job-failures:ws_1:owners:def:100"))
	assert.True(t, open("u1", "job-failures:ws_2:owners:abc:100"))

	key, err := ns.LatestGroupSince(ctx, "u1", "job-failures:ws_1:owners:abc:", now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, "job-failures:ws_1:owners:abc:100", key)

	key, err = ns.LatestGroupSince(ctx, "u1", "job-failures:ws_1:owners:abc:", now.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, key, "a group older than the window is closed")

	// An underscore in the prefix is a literal, never a LIKE wildcard: ws_1
	// must not match wsX1.
	assert.True(t, open("u1", "job-failures:wsX1:owners:abc:100"))
	n, err := ns.CountGroupsSince(ctx, "u1", "job-failures:ws_1:", now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	require.NoError(t, ns.MarkAllRead(ctx, "u1"))
	require.NoError(t, ns.RewriteGroup(ctx, "u1", "job-failures:ws_1:owners:abc:100",
		"2 translation jobs did not finish", "They stopped for the same reason.", "/acme"))
	unread, err := ns.List(ctx, "u1", 50, true)
	require.NoError(t, err)
	require.Len(t, unread, 1)
	assert.Equal(t, "2 translation jobs did not finish", unread[0].Title)
	assert.Equal(t, "/acme", unread[0].LinkURL)

	added, size, err := ns.AddGroupMember(ctx, "g", "job-1", now)
	require.NoError(t, err)
	assert.True(t, added)
	assert.Equal(t, 1, size)
	added, size, err = ns.AddGroupMember(ctx, "g", "job-1", now)
	require.NoError(t, err)
	assert.False(t, added, "a member is recorded once")
	assert.Equal(t, 1, size)
	_, size, err = ns.AddGroupMember(ctx, "g", "job-2", now)
	require.NoError(t, err)
	assert.Equal(t, 2, size)

	_, _, err = ns.AddGroupMember(ctx, "old", "job-0", now.Add(-48*time.Hour))
	require.NoError(t, err)
	require.NoError(t, ns.PruneGroupMembers(ctx, now.Add(-24*time.Hour)))
	size, err = ns.GroupSize(ctx, "old")
	require.NoError(t, err)
	assert.Zero(t, size)
	size, err = ns.GroupSize(ctx, "g")
	require.NoError(t, err)
	assert.Equal(t, 2, size)
}
