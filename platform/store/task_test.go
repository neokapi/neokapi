package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTaskStore(t *testing.T) *TaskStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return NewTaskStore(s.DB())
}

func TestTaskStore_CRUD(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := context.Background()

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
			Title:       "Review strings",
			CreatedBy:   "user-1",
		}
		require.NoError(t, store.Create(ctx, task))

		task.Title = "Review all strings"
		task.Priority = TaskPriorityUrgent
		err := store.Update(ctx, task)
		require.NoError(t, err)

		got, err := store.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, "Review all strings", got.Title)
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
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

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
