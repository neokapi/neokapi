package event

import (
	"context"
	"testing"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTaskStore(t *testing.T) *bstore.TaskStore {
	t.Helper()
	s, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return bstore.NewTaskStore(s.DB())
}

func newTestDispatcher(t *testing.T) (*NotificationDispatcher, *bstore.NotificationStore, *mockSender) {
	t.Helper()
	bus := NewChannelEventBus()
	t.Cleanup(func() { bus.Close() })

	notifStore := newTestNotifStore(t)
	sender := &mockSender{}

	d := NewNotificationDispatcher(bus, notifStore, nil, sender, nil)
	t.Cleanup(func() { d.Close() })

	return d, notifStore, sender
}

func TestDeadlineChecker_Check(t *testing.T) {
	taskStore := newTestTaskStore(t)
	dispatcher, notifStore, sender := newTestDispatcher(t)
	ctx := context.Background()

	// Create a task due within 24 hours (should trigger notification).
	dueIn12h := time.Now().UTC().Add(12 * time.Hour)
	err := taskStore.Create(ctx, &bstore.Task{
		ID:          "task-due-soon",
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        bstore.TaskTranslate,
		Status:      bstore.TaskStatusOpen,
		Priority:    bstore.TaskPriorityHigh,
		Title:       "Translate homepage",
		AssigneeID:  "user-1",
		CreatedBy:   "admin",
		DueAt:       &dueIn12h,
	})
	require.NoError(t, err)

	// Create a task due far in the future (should NOT trigger notification).
	dueFarAway := time.Now().UTC().Add(7 * 24 * time.Hour)
	err = taskStore.Create(ctx, &bstore.Task{
		ID:          "task-due-later",
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        bstore.TaskReview,
		Status:      bstore.TaskStatusOpen,
		Priority:    bstore.TaskPriorityNormal,
		Title:       "Review translations",
		AssigneeID:  "user-2",
		CreatedBy:   "admin",
		DueAt:       &dueFarAway,
	})
	require.NoError(t, err)

	dc := NewDeadlineChecker(taskStore, dispatcher, time.Hour)

	// Invoke the check method directly.
	dc.check()

	// Only user-1's task is due within 24h, so only one notification.
	assert.Equal(t, 1, sender.count())

	notifs, err := notifStore.List(ctx, "user-1", 10, false)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, bstore.NotificationDeadlineApproaching, notifs[0].Type)
	assert.Equal(t, "high", notifs[0].Priority)
	assert.Contains(t, notifs[0].Body, "Translate homepage")
	assert.Equal(t, "task-due-soon", notifs[0].TaskID)

	// user-2 should have no notifications.
	notifs2, err := notifStore.List(ctx, "user-2", 10, false)
	require.NoError(t, err)
	assert.Empty(t, notifs2)
}

func TestDeadlineChecker_CheckInProgressTasks(t *testing.T) {
	taskStore := newTestTaskStore(t)
	dispatcher, notifStore, sender := newTestDispatcher(t)
	ctx := context.Background()

	dueIn6h := time.Now().UTC().Add(6 * time.Hour)
	err := taskStore.Create(ctx, &bstore.Task{
		ID:          "task-in-progress",
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        bstore.TaskTranslate,
		Status:      bstore.TaskStatusInProgress,
		Priority:    bstore.TaskPriorityNormal,
		Title:       "In-progress task",
		AssigneeID:  "user-3",
		CreatedBy:   "admin",
		DueAt:       &dueIn6h,
	})
	require.NoError(t, err)

	dc := NewDeadlineChecker(taskStore, dispatcher, time.Hour)
	dc.check()

	assert.Equal(t, 1, sender.count())

	notifs, err := notifStore.List(ctx, "user-3", 10, false)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, bstore.NotificationDeadlineApproaching, notifs[0].Type)
	assert.Contains(t, notifs[0].Body, "In-progress task")
}

func TestDeadlineChecker_StartStop(t *testing.T) {
	taskStore := newTestTaskStore(t)
	dispatcher, _, _ := newTestDispatcher(t)

	dc := NewDeadlineChecker(taskStore, dispatcher, 10*time.Millisecond)
	dc.Start()
	// Give it a moment to start the goroutine and tick at least once.
	time.Sleep(50 * time.Millisecond)
	dc.Close()
	// Should not hang — the done channel should be closed.
}

func TestDeadlineChecker_SkipsTasksWithoutAssignee(t *testing.T) {
	taskStore := newTestTaskStore(t)
	dispatcher, notifStore, sender := newTestDispatcher(t)
	ctx := context.Background()

	dueIn3h := time.Now().UTC().Add(3 * time.Hour)
	err := taskStore.Create(ctx, &bstore.Task{
		ID:          "task-no-assignee",
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        bstore.TaskTranslate,
		Status:      bstore.TaskStatusOpen,
		Priority:    bstore.TaskPriorityNormal,
		Title:       "Unassigned task",
		AssigneeID:  "", // No assignee.
		CreatedBy:   "admin",
		DueAt:       &dueIn3h,
	})
	require.NoError(t, err)

	dc := NewDeadlineChecker(taskStore, dispatcher, time.Hour)
	dc.check()

	assert.Equal(t, 0, sender.count())

	// No notifications should exist for anyone.
	notifs, err := notifStore.List(ctx, "", 10, false)
	require.NoError(t, err)
	assert.Empty(t, notifs)
}

func TestDeadlineChecker_SkipsTasksWithoutDueAt(t *testing.T) {
	taskStore := newTestTaskStore(t)
	dispatcher, notifStore, sender := newTestDispatcher(t)
	ctx := context.Background()

	err := taskStore.Create(ctx, &bstore.Task{
		ID:          "task-no-due",
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		Type:        bstore.TaskTranslate,
		Status:      bstore.TaskStatusOpen,
		Priority:    bstore.TaskPriorityNormal,
		Title:       "No deadline task",
		AssigneeID:  "user-1",
		CreatedBy:   "admin",
		DueAt:       nil, // No due date.
	})
	require.NoError(t, err)

	dc := NewDeadlineChecker(taskStore, dispatcher, time.Hour)
	dc.check()

	// The task has no DueAt, so the query with DueBefore won't return it,
	// and even if it did, the check() method skips tasks without DueAt.
	assert.Equal(t, 0, sender.count())

	notifs, err := notifStore.List(ctx, "user-1", 10, false)
	require.NoError(t, err)
	assert.Empty(t, notifs)
}
