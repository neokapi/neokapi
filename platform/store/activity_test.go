package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestActivityStore(t *testing.T) *ActivityStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return NewActivityStore(s.DB())
}

func TestActivityStore_CreateAndList(t *testing.T) {
	store := newTestActivityStore(t)
	ctx := context.Background()

	t.Run("create activity", func(t *testing.T) {
		a := &Activity{
			WorkspaceID: "ws-1",
			ProjectID:   "proj-1",
			ActorID:     "user-1",
			ActorName:   "Alice",
			Type:        ActivityProjectCreated,
			EntityType:  "project",
			EntityID:    "proj-1",
			Summary:     "created project Test",
			Data:        map[string]string{"name": "Test"},
		}
		err := store.Create(ctx, a)
		require.NoError(t, err)
		assert.NotEmpty(t, a.ID)
		assert.False(t, a.CreatedAt.IsZero())
	})

	t.Run("list returns activities", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1"})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 1)
		assert.Equal(t, ActivityProjectCreated, result.Activities[0].Type)
		assert.Equal(t, "Alice", result.Activities[0].ActorName)
		assert.Equal(t, "Test", result.Activities[0].Data["name"])
	})

	t.Run("filter by project", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", ProjectID: "proj-1"})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 1)

		result, err = store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", ProjectID: "nonexistent"})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 0)
	})

	t.Run("filter by actor", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", ActorID: "user-1"})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 1)

		result, err = store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", ActorID: "user-other"})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 0)
	})

	t.Run("filter by type prefix", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", Type: "project"})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 1)

		result, err = store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", Type: "block"})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 0)
	})
}

func TestActivityStore_Pagination(t *testing.T) {
	store := newTestActivityStore(t)
	ctx := context.Background()

	// Create 5 activities with staggered timestamps.
	for i := 0; i < 5; i++ {
		a := &Activity{
			WorkspaceID: "ws-1",
			ProjectID:   "proj-1",
			ActorID:     "user-1",
			Type:        ActivityBlockTranslated,
			Summary:     "translated block",
			CreatedAt:   time.Now().UTC().Add(time.Duration(-5+i) * time.Second),
		}
		require.NoError(t, store.Create(ctx, a))
	}

	// Page 1: 3 items.
	result, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", Limit: 3})
	require.NoError(t, err)
	assert.Len(t, result.Activities, 3)
	assert.NotEmpty(t, result.NextCursor)

	// Page 2: remaining items.
	result2, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", Limit: 3, Cursor: result.NextCursor})
	require.NoError(t, err)
	assert.Len(t, result2.Activities, 2)
	assert.Empty(t, result2.NextCursor)
}

func TestActivityStore_NilData(t *testing.T) {
	store := newTestActivityStore(t)
	ctx := context.Background()

	a := &Activity{
		WorkspaceID: "ws-1",
		ActorID:     "user-1",
		Type:        ActivityFlowCompleted,
		Summary:     "flow completed",
	}
	require.NoError(t, store.Create(ctx, a))

	result, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, result.Activities, 1)
	assert.NotNil(t, result.Activities[0].Data)
}
