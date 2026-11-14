package store

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestActivityStore(t *testing.T) *ActivityStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	_, err := NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return NewActivityStore(db.DB)
}

func TestActivityStore_CreateAndList(t *testing.T) {
	store := newTestActivityStore(t)
	ctx := t.Context()

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
		assert.Empty(t, result.Activities)
	})

	t.Run("filter by actor", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", ActorID: "user-1"})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 1)

		result, err = store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", ActorID: "user-other"})
		require.NoError(t, err)
		assert.Empty(t, result.Activities)
	})

	t.Run("filter by type prefix", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", Type: "project"})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 1)

		result, err = store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", Type: "block"})
		require.NoError(t, err)
		assert.Empty(t, result.Activities)
	})
}

func TestActivityStore_Pagination(t *testing.T) {
	store := newTestActivityStore(t)
	ctx := t.Context()

	// Create 5 activities with staggered timestamps.
	for i := range 5 {
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
	ctx := t.Context()

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

func TestActivityStore_TypesFilter(t *testing.T) {
	store := newTestActivityStore(t)
	ctx := t.Context()

	for _, typ := range []ActivityType{
		ActivityReviewAssigned, ActivityReviewDecided,
		ActivityTaskCreated, ActivityTaskCompleted,
		ActivityBlockTranslated, ActivityFlowCompleted,
	} {
		require.NoError(t, store.Create(ctx, &Activity{
			WorkspaceID: "ws-1", ActorID: "u1", Type: typ, Summary: string(typ),
		}))
	}

	types := func(as []Activity) []string {
		out := make([]string, 0, len(as))
		for _, a := range as {
			out = append(out, string(a.Type))
		}
		return out
	}

	t.Run("ORs one prefix per entry", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{
			WorkspaceID: "ws-1",
			Types:       []string{"review", "task"},
		})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{
			"review.assigned", "review.decided", "task.created", "task.completed",
		}, types(result.Activities))
	})

	t.Run("an exact type matches only itself", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{
			WorkspaceID: "ws-1",
			Types:       []string{"review.decided"},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"review.decided"}, types(result.Activities))
	})

	t.Run("types overrides the single prefix", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{
			WorkspaceID: "ws-1",
			Type:        "block",
			Types:       []string{"flow"},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"flow.completed"}, types(result.Activities))
	})

	t.Run("all-empty types is no filter", func(t *testing.T) {
		result, err := store.List(ctx, ActivityQuery{
			WorkspaceID: "ws-1",
			Types:       []string{""},
		})
		require.NoError(t, err)
		assert.Len(t, result.Activities, 6)
	})
}

// Rows that share a created_at across a page boundary are each returned once.
// Paging on created_at alone skipped every row the page did not reach.
func TestActivityStore_PagesRowsSharingATimestamp(t *testing.T) {
	store := newTestActivityStore(t)
	ctx := t.Context()

	at := time.Date(2026, 11, 14, 9, 30, 0, 0, time.UTC)
	const total = 23
	var want []string
	for range total {
		a := &Activity{
			WorkspaceID: "ws-1", ActorID: "user-1",
			Type: ActivityProjectUpdated, Summary: "updated", CreatedAt: at,
		}
		require.NoError(t, store.Create(ctx, a))
		want = append(want, a.ID)
	}

	seen := map[string]int{}
	cursor := ""
	for pages := 0; ; pages++ {
		require.Less(t, pages, total, "paging did not terminate")
		res, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1", Limit: 5, Cursor: cursor})
		require.NoError(t, err)
		for _, a := range res.Activities {
			seen[a.ID]++
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}

	assert.Len(t, seen, total)
	for _, id := range want {
		assert.Equal(t, 1, seen[id], "activity %s", id)
	}
}

func TestActivityStore_KeepsSubSecondTime(t *testing.T) {
	store := newTestActivityStore(t)
	ctx := t.Context()

	at := time.Date(2026, 11, 14, 9, 30, 0, 123456789, time.UTC)
	a := &Activity{WorkspaceID: "ws-1", ActorID: "u", Type: ActivityProjectUpdated, CreatedAt: at}
	require.NoError(t, store.Create(ctx, a))
	assert.Equal(t, at.Truncate(time.Microsecond), a.CreatedAt)

	res, err := store.List(ctx, ActivityQuery{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, res.Activities, 1)
	assert.True(t, a.CreatedAt.Equal(res.Activities[0].CreatedAt),
		"stored %s, read %s", a.CreatedAt, res.Activities[0].CreatedAt)
}

func TestActivityStore_RejectsForeignCursor(t *testing.T) {
	store := newTestActivityStore(t)
	for _, c := range []string{"2026-11-14T09:30:00Z", "not base64!", "bm8tc2VwYXJhdG9y"} {
		_, err := store.List(t.Context(), ActivityQuery{WorkspaceID: "ws-1", Cursor: c})
		assert.ErrorIs(t, err, ErrInvalidActivityCursor, c)
	}
}

// A workspace's rows are filed under its slug or its id. A query naming both
// lists both, and the daily counts and the unseen count agree with it.
func TestActivityStore_MatchesEveryWorkspaceKey(t *testing.T) {
	store := newTestActivityStore(t)
	ctx := t.Context()

	for _, key := range []string{"acme", "ws-uuid-1", "other"} {
		require.NoError(t, store.Create(ctx, &Activity{
			WorkspaceID: key, ActorID: "u", Type: ActivityProjectUpdated, Summary: key,
		}))
	}

	res, err := store.List(ctx, ActivityQuery{WorkspaceID: "acme", WorkspaceKeys: []string{"ws-uuid-1"}})
	require.NoError(t, err)
	assert.Len(t, res.Activities, 2)

	counts, err := store.DailyCounts(ctx, time.Now().Add(-time.Hour), "acme", "ws-uuid-1")
	require.NoError(t, err)
	total := 0
	for _, c := range counts {
		total += c.Count
	}
	assert.Equal(t, 2, total)

	require.NoError(t, store.SetActivitySeenAt(ctx, "u", "ws-uuid-1", time.Now().Add(-time.Hour)))
	n, err := store.CountNewActivities(ctx, "u", "ws-uuid-1", "acme")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}
