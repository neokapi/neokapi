package store

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPreferenceStore(t *testing.T) *PreferenceStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	_, err := NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return NewPreferenceStore(db.DB)
}

func TestPreferenceStore_DefaultPreferences(t *testing.T) {
	prefs := DefaultPreferences("user-1", "ws-1")
	assert.Len(t, prefs, 7) // 7 categories
	for _, p := range prefs {
		assert.Equal(t, "user-1", p.UserID)
		assert.Equal(t, "ws-1", p.WorkspaceID)
		assert.NotEmpty(t, string(p.Category))
	}

	// Task category should default to web=true.
	var found bool
	for _, p := range prefs {
		if p.Category == CategoryTask {
			assert.True(t, p.Web)
			found = true
		}
	}
	assert.True(t, found)
}

func TestPreferenceStore_UpsertAndGet(t *testing.T) {
	store := newTestPreferenceStore(t)
	ctx := t.Context()

	t.Run("upsert new preference", func(t *testing.T) {
		pref := NotificationPreference{
			UserID:      "user-1",
			WorkspaceID: "ws-1",
			Category:    CategoryTask,
			Web:         true,
			Email:       true,
			Push:        false,
			Desktop:     false,
		}
		err := store.Upsert(ctx, &pref)
		require.NoError(t, err)
	})

	t.Run("get returns stored preference", func(t *testing.T) {
		got, err := store.Get(ctx, "user-1", "ws-1", CategoryTask)
		require.NoError(t, err)
		assert.True(t, got.Web)
		assert.True(t, got.Email)
		assert.False(t, got.Push)
	})

	t.Run("upsert updates existing", func(t *testing.T) {
		pref := NotificationPreference{
			UserID:      "user-1",
			WorkspaceID: "ws-1",
			Category:    CategoryTask,
			Web:         false,
			Email:       false,
			Push:        true,
			Desktop:     true,
		}
		require.NoError(t, store.Upsert(ctx, &pref))

		got, err := store.Get(ctx, "user-1", "ws-1", CategoryTask)
		require.NoError(t, err)
		assert.False(t, got.Web)
		assert.True(t, got.Push)
		assert.True(t, got.Desktop)
	})
}

func TestPreferenceStore_ListMergesDefaults(t *testing.T) {
	store := newTestPreferenceStore(t)
	ctx := t.Context()

	// Upsert only one category.
	pref := NotificationPreference{
		UserID:      "user-1",
		WorkspaceID: "ws-1",
		Category:    CategoryQuality,
		Web:         false,
		Email:       true,
	}
	require.NoError(t, store.Upsert(ctx, &pref))

	prefs, err := store.List(ctx, "user-1", "ws-1")
	require.NoError(t, err)
	// Should return all 7 categories with defaults merged.
	assert.Len(t, prefs, 7)

	// The quality category should reflect stored values.
	var qualityPref *NotificationPreference
	for i := range prefs {
		if prefs[i].Category == CategoryQuality {
			qualityPref = &prefs[i]
			break
		}
	}
	require.NotNil(t, qualityPref)
	assert.False(t, qualityPref.Web)
	assert.True(t, qualityPref.Email)
}

func TestPreferenceStore_BulkUpsert(t *testing.T) {
	store := newTestPreferenceStore(t)
	ctx := t.Context()

	prefs := []NotificationPreference{
		{UserID: "user-1", WorkspaceID: "ws-1", Category: CategoryTask, Web: true, Email: true},
		{UserID: "user-1", WorkspaceID: "ws-1", Category: CategoryMention, Web: true, Push: true},
		{UserID: "user-1", WorkspaceID: "ws-1", Category: CategorySystem, Web: false},
	}
	err := store.BulkUpsert(ctx, prefs)
	require.NoError(t, err)

	all, err := store.List(ctx, "user-1", "ws-1")
	require.NoError(t, err)
	assert.Len(t, all, 7)

	// Verify task preference.
	for _, p := range all {
		if p.Category == CategoryTask {
			assert.True(t, p.Web)
			assert.True(t, p.Email)
		}
	}
}

func TestPreferenceStore_GetNonexistent_ReturnsDefault(t *testing.T) {
	store := newTestPreferenceStore(t)
	ctx := t.Context()

	got, err := store.Get(ctx, "user-1", "ws-1", CategoryTask)
	require.NoError(t, err) // Get returns a default when no explicit preference exists.
	assert.Equal(t, CategoryTask, got.Category)
	assert.True(t, got.Web) // Default for task category is web=true.
}
