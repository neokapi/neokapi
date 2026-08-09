package store

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTaskStore(t *testing.T) *TaskStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	_, err := NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return NewTaskStore(db.DB)
}

func TestTaskStore_CRUD(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	t.Run("create and get", func(t *testing.T) {
		task := &Task{
			WorkspaceID: "ws-1",
			ProjectID:   "proj-1",
			Type:        TaskTranslate,
			Priority:    TaskPriorityHigh,
			Title:       "Translate homepage",
			Description: "French translation needed",
			AssigneeID:  "user-2",
			CreatedBy:   "user-1",
		}
		err := store.Create(ctx, task)
		require.NoError(t, err)
		assert.NotEmpty(t, task.ID)
		assert.Equal(t, TaskStatusOpen, task.Status)

		got, err := store.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, task.Title, got.Title)
		assert.Equal(t, TaskPriorityHigh, got.Priority)
		assert.Equal(t, "user-2", got.AssigneeID)
	})

	t.Run("update", func(t *testing.T) {
		task := &Task{
			WorkspaceID: "ws-1",
			ProjectID:   "proj-1",
			Type:        TaskReview,
			Title:       "Review blocks",
			CreatedBy:   "user-1",
		}
		require.NoError(t, store.Create(ctx, task))

		task.Title = "Review all blocks"
		task.Priority = TaskPriorityUrgent
		err := store.Update(ctx, task)
		require.NoError(t, err)

		got, err := store.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, "Review all blocks", got.Title)
		assert.Equal(t, TaskPriorityUrgent, got.Priority)
	})

	t.Run("delete", func(t *testing.T) {
		task := &Task{
			WorkspaceID: "ws-1",
			ProjectID:   "proj-1",
			Type:        TaskCustom,
			Title:       "Deletable task",
			CreatedBy:   "user-1",
		}
		require.NoError(t, store.Create(ctx, task))

		err := store.Delete(ctx, task.ID)
		require.NoError(t, err)

		_, err = store.Get(ctx, task.ID)
		assert.Error(t, err)
	})
}

func TestTaskStore_Lifecycle(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	task := &Task{
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        TaskTranslate,
		Title:       "Lifecycle test",
		CreatedBy:   "user-1",
	}
	require.NoError(t, store.Create(ctx, task))
	assert.Equal(t, TaskStatusOpen, task.Status)

	t.Run("assign sets in_progress", func(t *testing.T) {
		err := store.Assign(ctx, task.ID, "user-2")
		require.NoError(t, err)

		got, err := store.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, TaskStatusInProgress, got.Status)
		assert.Equal(t, "user-2", got.AssigneeID)
	})

	t.Run("complete sets completed", func(t *testing.T) {
		err := store.Complete(ctx, task.ID, "user-2")
		require.NoError(t, err)

		got, err := store.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, TaskStatusCompleted, got.Status)
		assert.Equal(t, "user-2", got.CompletedBy)
		assert.NotNil(t, got.CompletedAt)
	})

	t.Run("cannot complete already completed task", func(t *testing.T) {
		// Complete is a no-op for already completed tasks (WHERE status IN clause).
		err := store.Complete(ctx, task.ID, "user-3")
		require.NoError(t, err)

		got, err := store.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, "user-2", got.CompletedBy) // unchanged
	})
}

func TestTaskStore_Cancel(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	task := &Task{
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        TaskReview,
		Title:       "Cancellable task",
		CreatedBy:   "user-1",
	}
	require.NoError(t, store.Create(ctx, task))

	err := store.Cancel(ctx, task.ID)
	require.NoError(t, err)

	got, err := store.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, TaskStatusCancelled, got.Status)
}

func TestTaskStore_ListFilters(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	// Create tasks with different attributes.
	tasks := []*Task{
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskTranslate, Title: "T1", AssigneeID: "user-1", CreatedBy: "user-0", Priority: TaskPriorityHigh},
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskReview, Title: "T2", AssigneeID: "user-2", CreatedBy: "user-0", Priority: TaskPriorityNormal},
		{WorkspaceID: "ws-1", ProjectID: "proj-2", Type: TaskTranslate, Title: "T3", AssigneeID: "user-1", CreatedBy: "user-0", Priority: TaskPriorityUrgent},
	}
	for _, task := range tasks {
		require.NoError(t, store.Create(ctx, task))
	}

	t.Run("filter by workspace", func(t *testing.T) {
		result, err := store.List(ctx, TaskQuery{WorkspaceID: "ws-1"})
		require.NoError(t, err)
		assert.Len(t, result.Tasks, 3)
	})

	t.Run("filter by project", func(t *testing.T) {
		result, err := store.List(ctx, TaskQuery{WorkspaceID: "ws-1", ProjectID: "proj-1"})
		require.NoError(t, err)
		assert.Len(t, result.Tasks, 2)
	})

	t.Run("filter by assignee", func(t *testing.T) {
		result, err := store.List(ctx, TaskQuery{WorkspaceID: "ws-1", AssigneeID: "user-1"})
		require.NoError(t, err)
		assert.Len(t, result.Tasks, 2)
	})

	t.Run("filter by type", func(t *testing.T) {
		result, err := store.List(ctx, TaskQuery{WorkspaceID: "ws-1", Type: string(TaskReview)})
		require.NoError(t, err)
		assert.Len(t, result.Tasks, 1)
		assert.Equal(t, "T2", result.Tasks[0].Title)
	})

	t.Run("filter by priority", func(t *testing.T) {
		result, err := store.List(ctx, TaskQuery{WorkspaceID: "ws-1", Priority: string(TaskPriorityUrgent)})
		require.NoError(t, err)
		assert.Len(t, result.Tasks, 1)
		assert.Equal(t, "T3", result.Tasks[0].Title)
	})
}

func TestTaskStore_DueAt(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	due := time.Now().UTC().Add(24 * time.Hour)
	task := &Task{
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        TaskTranslate,
		Title:       "Due task",
		CreatedBy:   "user-1",
		DueAt:       &due,
	}
	require.NoError(t, store.Create(ctx, task))

	got, err := store.Get(ctx, task.ID)
	require.NoError(t, err)
	require.NotNil(t, got.DueAt)
	assert.WithinDuration(t, due, *got.DueAt, time.Second)
}

func TestTaskStore_DueBeforeFilter(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	now := time.Now().UTC()
	dueIn12h := now.Add(12 * time.Hour)
	dueIn48h := now.Add(48 * time.Hour)

	// Task due in 12h (within 24h horizon).
	t1 := &Task{
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        TaskTranslate,
		Title:       "Due soon",
		CreatedBy:   "user-1",
		AssigneeID:  "user-2",
		DueAt:       &dueIn12h,
	}
	require.NoError(t, store.Create(ctx, t1))

	// Task due in 48h (outside 24h horizon).
	t2 := &Task{
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        TaskTranslate,
		Title:       "Due later",
		CreatedBy:   "user-1",
		AssigneeID:  "user-2",
		DueAt:       &dueIn48h,
	}
	require.NoError(t, store.Create(ctx, t2))

	// Task with no due date.
	t3 := &Task{
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        TaskTranslate,
		Title:       "No deadline",
		CreatedBy:   "user-1",
		AssigneeID:  "user-2",
	}
	require.NoError(t, store.Create(ctx, t3))

	horizon := now.Add(24 * time.Hour)
	result, err := store.List(ctx, TaskQuery{
		DueBefore: &horizon,
	})
	require.NoError(t, err)
	assert.Len(t, result.Tasks, 1)
	assert.Equal(t, "Due soon", result.Tasks[0].Title)
}

func TestTaskStore_DueBeforeFilter_IncludesExactMatch(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	due := time.Now().UTC().Add(24 * time.Hour)
	task := &Task{
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        TaskTranslate,
		Title:       "Exact deadline",
		CreatedBy:   "user-1",
		DueAt:       &due,
	}
	require.NoError(t, store.Create(ctx, task))

	// Query with DueBefore exactly at the task's due time.
	result, err := store.List(ctx, TaskQuery{
		DueBefore: &due,
	})
	require.NoError(t, err)
	assert.Len(t, result.Tasks, 1)
	assert.Equal(t, "Exact deadline", result.Tasks[0].Title)
}

func TestTaskStore_TypesFilter(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	seed := []*Task{
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskTranslate, Title: "T-translate", CreatedBy: "u0"},
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskReview, Title: "T-review", CreatedBy: "u0"},
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskReviewTerms, Title: "T-review-terms", CreatedBy: "u0"},
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskSourceReview, Title: "T-source-review", CreatedBy: "u0"},
		{WorkspaceID: "ws-2", ProjectID: "proj-9", Type: TaskReview, Title: "other-workspace", CreatedBy: "u0"},
	}
	for _, task := range seed {
		require.NoError(t, store.Create(ctx, task))
	}

	titles := func(tasks []Task) []string {
		out := make([]string, 0, len(tasks))
		for _, task := range tasks {
			out = append(out, task.Title)
		}
		return out
	}

	t.Run("matches any of the listed types", func(t *testing.T) {
		result, err := store.List(ctx, TaskQuery{
			WorkspaceID: "ws-1",
			Types: []string{
				string(TaskReview), string(TaskReviewTerms), string(TaskSourceReview),
			},
		})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"T-review", "T-review-terms", "T-source-review"}, titles(result.Tasks))
	})

	t.Run("stays inside the workspace", func(t *testing.T) {
		result, err := store.List(ctx, TaskQuery{
			WorkspaceID: "ws-2",
			Types:       []string{string(TaskReview)},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"other-workspace"}, titles(result.Tasks))
	})

	t.Run("types overrides the single type", func(t *testing.T) {
		result, err := store.List(ctx, TaskQuery{
			WorkspaceID: "ws-1",
			Type:        string(TaskTranslate),
			Types:       []string{string(TaskReview)},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"T-review"}, titles(result.Tasks))
	})

	t.Run("empty types falls back to the single type", func(t *testing.T) {
		result, err := store.List(ctx, TaskQuery{
			WorkspaceID: "ws-1",
			Type:        string(TaskTranslate),
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"T-translate"}, titles(result.Tasks))
	})
}

func TestTaskStore_Counts(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	seed := []*Task{
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskReview, Title: "open-1", CreatedBy: "u0"},
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskReview, Title: "open-2", CreatedBy: "u0"},
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskReview, Title: "in-progress", CreatedBy: "u0"},
		{WorkspaceID: "ws-1", ProjectID: "proj-1", Type: TaskTranslate, Title: "translate", CreatedBy: "u0"},
		{WorkspaceID: "ws-2", ProjectID: "proj-9", Type: TaskReview, Title: "other-workspace", CreatedBy: "u0"},
	}
	for _, task := range seed {
		require.NoError(t, store.Create(ctx, task))
	}
	require.NoError(t, store.Assign(ctx, seed[2].ID, "u1"))
	require.NoError(t, store.Complete(ctx, seed[3].ID, "u1"))

	t.Run("groups the workspace by status", func(t *testing.T) {
		counts, err := store.Counts(ctx, TaskQuery{WorkspaceID: "ws-1"})
		require.NoError(t, err)
		assert.Equal(t, 4, counts.Total)
		assert.Equal(t, 2, counts.ByStatus[string(TaskStatusOpen)])
		assert.Equal(t, 1, counts.ByStatus[string(TaskStatusInProgress)])
		assert.Equal(t, 1, counts.ByStatus[string(TaskStatusCompleted)])
		assert.Equal(t, 0, counts.ByStatus[string(TaskStatusCancelled)])
	})

	t.Run("honours the same filters as List", func(t *testing.T) {
		counts, err := store.Counts(ctx, TaskQuery{
			WorkspaceID: "ws-1",
			Types:       []string{string(TaskReview)},
		})
		require.NoError(t, err)
		assert.Equal(t, 3, counts.Total)
		assert.Equal(t, 2, counts.ByStatus[string(TaskStatusOpen)])
		assert.Equal(t, 1, counts.ByStatus[string(TaskStatusInProgress)])
		assert.Equal(t, 0, counts.ByStatus[string(TaskStatusCompleted)])
	})

	t.Run("counts the whole set, not the page", func(t *testing.T) {
		counts, err := store.Counts(ctx, TaskQuery{WorkspaceID: "ws-1", Limit: 1})
		require.NoError(t, err)
		assert.Equal(t, 4, counts.Total)
	})

	t.Run("every known status is present, zero-filled", func(t *testing.T) {
		counts, err := store.Counts(ctx, TaskQuery{WorkspaceID: "ws-nothing"})
		require.NoError(t, err)
		assert.Equal(t, 0, counts.Total)
		assert.Len(t, counts.ByStatus, 4)
	})
}
