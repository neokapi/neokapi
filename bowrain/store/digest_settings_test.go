package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDigestStore(t *testing.T) *DigestStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return NewDigestStore(s.DB())
}

func TestDigestStore_GetSettings_DefaultsWhenEmpty(t *testing.T) {
	store := newTestDigestStore(t)
	ctx := t.Context()

	ds, err := store.GetSettings(ctx, "user-1", "ws-1")
	require.NoError(t, err)
	assert.Equal(t, "user-1", ds.UserID)
	assert.Equal(t, "ws-1", ds.WorkspaceID)
	assert.Equal(t, DigestDaily, ds.Frequency)
	assert.Equal(t, "UTC", ds.Timezone)
	assert.Empty(t, ds.QuietStart)
	assert.Empty(t, ds.QuietEnd)
}

func TestDigestStore_UpsertSettings(t *testing.T) {
	store := newTestDigestStore(t)
	ctx := t.Context()

	t.Run("create new settings", func(t *testing.T) {
		ds := &DigestSettings{
			UserID:      "user-1",
			WorkspaceID: "ws-1",
			Frequency:   DigestWeekly,
			QuietStart:  "22:00",
			QuietEnd:    "08:00",
			Timezone:    "America/New_York",
		}
		require.NoError(t, store.UpsertSettings(ctx, ds))

		got, err := store.GetSettings(ctx, "user-1", "ws-1")
		require.NoError(t, err)
		assert.Equal(t, DigestWeekly, got.Frequency)
		assert.Equal(t, "22:00", got.QuietStart)
		assert.Equal(t, "08:00", got.QuietEnd)
		assert.Equal(t, "America/New_York", got.Timezone)
	})

	t.Run("update existing settings", func(t *testing.T) {
		ds := &DigestSettings{
			UserID:      "user-1",
			WorkspaceID: "ws-1",
			Frequency:   DigestOff,
			QuietStart:  "",
			QuietEnd:    "",
			Timezone:    "Europe/Berlin",
		}
		require.NoError(t, store.UpsertSettings(ctx, ds))

		got, err := store.GetSettings(ctx, "user-1", "ws-1")
		require.NoError(t, err)
		assert.Equal(t, DigestOff, got.Frequency)
		assert.Empty(t, got.QuietStart)
		assert.Empty(t, got.QuietEnd)
		assert.Equal(t, "Europe/Berlin", got.Timezone)
	})
}

func TestDigestStore_ListUsersWithFrequency(t *testing.T) {
	store := newTestDigestStore(t)
	ctx := t.Context()

	// Insert settings with different frequencies.
	settings := []DigestSettings{
		{UserID: "user-1", WorkspaceID: "ws-1", Frequency: DigestDaily, Timezone: "UTC"},
		{UserID: "user-2", WorkspaceID: "ws-1", Frequency: DigestDaily, Timezone: "UTC"},
		{UserID: "user-3", WorkspaceID: "ws-1", Frequency: DigestWeekly, Timezone: "UTC"},
		{UserID: "user-4", WorkspaceID: "ws-2", Frequency: DigestOff, Timezone: "UTC"},
	}
	for i := range settings {
		require.NoError(t, store.UpsertSettings(ctx, &settings[i]))
	}

	t.Run("filter daily", func(t *testing.T) {
		result, err := store.ListUsersWithFrequency(ctx, DigestDaily)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		ids := []string{result[0].UserID, result[1].UserID}
		assert.Contains(t, ids, "user-1")
		assert.Contains(t, ids, "user-2")
	})

	t.Run("filter weekly", func(t *testing.T) {
		result, err := store.ListUsersWithFrequency(ctx, DigestWeekly)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "user-3", result[0].UserID)
	})

	t.Run("filter off", func(t *testing.T) {
		result, err := store.ListUsersWithFrequency(ctx, DigestOff)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "user-4", result[0].UserID)
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		// All users have explicit settings; query a frequency with no matches after clearing.
		// But since we have all three covered, just verify the counts are correct.
		result, err := store.ListUsersWithFrequency(ctx, DigestFrequency("nonexistent"))
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestDigestStore_GetState_DefaultWhenEmpty(t *testing.T) {
	store := newTestDigestStore(t)
	ctx := t.Context()

	before := time.Now().UTC().Add(-24 * time.Hour)
	state, err := store.GetState(ctx, "user-1", "ws-1", "daily")
	require.NoError(t, err)
	assert.Equal(t, "user-1", state.UserID)
	assert.Equal(t, "ws-1", state.WorkspaceID)
	assert.Equal(t, "daily", state.Frequency)
	// Default last_sent_at should be approximately 24h ago.
	assert.True(t, state.LastSentAt.After(before) || state.LastSentAt.Equal(before))
	assert.True(t, state.LastSentAt.Before(time.Now().UTC()))
}

func TestDigestStore_UpdateState(t *testing.T) {
	store := newTestDigestStore(t)
	ctx := t.Context()

	t.Run("create state", func(t *testing.T) {
		sentAt := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
		require.NoError(t, store.UpdateState(ctx, "user-1", "ws-1", "daily", sentAt))

		state, err := store.GetState(ctx, "user-1", "ws-1", "daily")
		require.NoError(t, err)
		assert.Equal(t, sentAt.Format(time.RFC3339), state.LastSentAt.Format(time.RFC3339))
	})

	t.Run("update existing state", func(t *testing.T) {
		newSentAt := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
		require.NoError(t, store.UpdateState(ctx, "user-1", "ws-1", "daily", newSentAt))

		state, err := store.GetState(ctx, "user-1", "ws-1", "daily")
		require.NoError(t, err)
		assert.Equal(t, newSentAt.Format(time.RFC3339), state.LastSentAt.Format(time.RFC3339))
	})
}

func TestDigestStore_IsInQuietHours(t *testing.T) {
	store := newTestDigestStore(t)

	t.Run("normal range inside", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "09:00",
			QuietEnd:   "17:00",
			Timezone:   "UTC",
		}
		// 12:00 UTC is inside 09:00-17:00
		now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
		assert.True(t, store.IsInQuietHours(ds, now))
	})

	t.Run("normal range outside", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "09:00",
			QuietEnd:   "17:00",
			Timezone:   "UTC",
		}
		// 08:00 UTC is outside 09:00-17:00
		now := time.Date(2026, 3, 26, 8, 0, 0, 0, time.UTC)
		assert.False(t, store.IsInQuietHours(ds, now))
	})

	t.Run("normal range at boundary start", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "09:00",
			QuietEnd:   "17:00",
			Timezone:   "UTC",
		}
		// Exactly 09:00 should be inside (>=)
		now := time.Date(2026, 3, 26, 9, 0, 0, 0, time.UTC)
		assert.True(t, store.IsInQuietHours(ds, now))
	})

	t.Run("normal range at boundary end", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "09:00",
			QuietEnd:   "17:00",
			Timezone:   "UTC",
		}
		// Exactly 17:00 should be outside (<)
		now := time.Date(2026, 3, 26, 17, 0, 0, 0, time.UTC)
		assert.False(t, store.IsInQuietHours(ds, now))
	})

	t.Run("overnight range inside late", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "22:00",
			QuietEnd:   "08:00",
			Timezone:   "UTC",
		}
		// 23:00 UTC is inside 22:00-08:00
		now := time.Date(2026, 3, 26, 23, 0, 0, 0, time.UTC)
		assert.True(t, store.IsInQuietHours(ds, now))
	})

	t.Run("overnight range inside early", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "22:00",
			QuietEnd:   "08:00",
			Timezone:   "UTC",
		}
		// 05:00 UTC is inside 22:00-08:00
		now := time.Date(2026, 3, 26, 5, 0, 0, 0, time.UTC)
		assert.True(t, store.IsInQuietHours(ds, now))
	})

	t.Run("overnight range outside", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "22:00",
			QuietEnd:   "08:00",
			Timezone:   "UTC",
		}
		// 12:00 UTC is outside 22:00-08:00
		now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
		assert.False(t, store.IsInQuietHours(ds, now))
	})

	t.Run("empty fields returns false", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "",
			QuietEnd:   "",
			Timezone:   "UTC",
		}
		now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
		assert.False(t, store.IsInQuietHours(ds, now))
	})

	t.Run("empty start only returns false", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "",
			QuietEnd:   "17:00",
			Timezone:   "UTC",
		}
		now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
		assert.False(t, store.IsInQuietHours(ds, now))
	})

	t.Run("invalid timezone returns false", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "09:00",
			QuietEnd:   "17:00",
			Timezone:   "Invalid/Timezone",
		}
		now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
		assert.False(t, store.IsInQuietHours(ds, now))
	})

	t.Run("timezone conversion", func(t *testing.T) {
		ds := &DigestSettings{
			QuietStart: "09:00",
			QuietEnd:   "17:00",
			Timezone:   "America/New_York",
		}
		// 14:00 UTC = 10:00 ET (inside 09:00-17:00 ET)
		now := time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC)
		assert.True(t, store.IsInQuietHours(ds, now))

		// 05:00 UTC = 01:00 ET (outside 09:00-17:00 ET)
		now = time.Date(2026, 3, 26, 5, 0, 0, 0, time.UTC)
		assert.False(t, store.IsInQuietHours(ds, now))
	})
}
