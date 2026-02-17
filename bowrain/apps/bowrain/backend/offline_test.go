package backend

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestQueue(t *testing.T) *OfflineQueue {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test-queue.db")
	q, err := NewOfflineQueue(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { q.Close() })
	return q
}

func TestOfflineQueueEnqueueAndCount(t *testing.T) {
	q := newTestQueue(t)

	assert.Equal(t, 0, q.PendingCount())

	err := q.Enqueue("update_block_target", map[string]string{
		"project_id": "p1", "block_id": "b1", "target_locale": "fr", "text": "Bonjour",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, q.PendingCount())

	err = q.Enqueue("review_block", map[string]string{
		"project_id": "p1", "block_id": "b1",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, q.PendingCount())
}

func TestOfflineQueuePeekPendingFIFO(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue("op1", map[string]string{"key": "first"})
	_ = q.Enqueue("op2", map[string]string{"key": "second"})
	_ = q.Enqueue("op3", map[string]string{"key": "third"})

	changes, err := q.PeekPending(2)
	require.NoError(t, err)
	require.Len(t, changes, 2)

	// FIFO: first enqueued should come first.
	assert.Equal(t, "op1", changes[0].Operation)
	assert.Equal(t, "op2", changes[1].Operation)
	assert.Equal(t, "pending", changes[0].Status)
}

func TestOfflineQueuePeekPendingPayload(t *testing.T) {
	q := newTestQueue(t)

	payload := UpdateBlockRequest{
		ProjectID:    "p1",
		ItemName:     "hello.txt",
		BlockID:      "b1",
		TargetLocale: "fr",
		Text:         "Bonjour le monde",
	}
	err := q.Enqueue("update_block_target", payload)
	require.NoError(t, err)

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)

	// Payload should be valid JSON matching the original struct.
	var decoded UpdateBlockRequest
	err = json.Unmarshal([]byte(changes[0].Payload), &decoded)
	require.NoError(t, err)
	assert.Equal(t, "p1", decoded.ProjectID)
	assert.Equal(t, "hello.txt", decoded.ItemName)
	assert.Equal(t, "b1", decoded.BlockID)
	assert.Equal(t, "fr", decoded.TargetLocale)
	assert.Equal(t, "Bonjour le monde", decoded.Text)
}

func TestOfflineQueueMarkCompleted(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue("op1", nil)
	_ = q.Enqueue("op2", nil)

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 2)

	// Mark first as completed.
	err = q.MarkCompleted(changes[0].ID)
	require.NoError(t, err)

	// Only one pending now.
	assert.Equal(t, 1, q.PendingCount())

	// PeekPending should only return the second.
	remaining, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "op2", remaining[0].Operation)
}

func TestOfflineQueueMarkFailed(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue("op1", nil)
	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)

	// Mark as failed — it should still be pending (retriable).
	err = q.MarkFailed(changes[0].ID, "network timeout")
	require.NoError(t, err)

	assert.Equal(t, 1, q.PendingCount())

	// Peek again to verify attempts and error.
	changes, err = q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, 1, changes[0].Attempts)
	assert.Equal(t, "network timeout", changes[0].LastError)

	// Mark failed again.
	err = q.MarkFailed(changes[0].ID, "connection refused")
	require.NoError(t, err)

	changes, err = q.PeekPending(10)
	require.NoError(t, err)
	assert.Equal(t, 2, changes[0].Attempts)
	assert.Equal(t, "connection refused", changes[0].LastError)
}

func TestOfflineQueuePurgeCompleted(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue("op1", nil)
	_ = q.Enqueue("op2", nil)
	_ = q.Enqueue("op3", nil)

	changes, err := q.PeekPending(10)
	require.NoError(t, err)

	// Complete the first two.
	_ = q.MarkCompleted(changes[0].ID)
	_ = q.MarkCompleted(changes[1].ID)

	err = q.PurgeCompleted()
	require.NoError(t, err)

	// Only one pending remains.
	assert.Equal(t, 1, q.PendingCount())

	remaining, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "op3", remaining[0].Operation)
}

func TestOfflineQueueClear(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue("op1", nil)
	_ = q.Enqueue("op2", nil)
	_ = q.Enqueue("op3", nil)
	assert.Equal(t, 3, q.PendingCount())

	err := q.Clear()
	require.NoError(t, err)
	assert.Equal(t, 0, q.PendingCount())
}

func TestOfflineQueueEmptyPeek(t *testing.T) {
	q := newTestQueue(t)

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	assert.Empty(t, changes)
}

func TestOfflineQueuePersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist-queue.db")

	// Create queue and enqueue.
	q1, err := NewOfflineQueue(dbPath)
	require.NoError(t, err)

	_ = q1.Enqueue("op1", map[string]string{"key": "value"})
	_ = q1.Enqueue("op2", nil)
	q1.Close()

	// Reopen and verify data persists.
	q2, err := NewOfflineQueue(dbPath)
	require.NoError(t, err)
	defer q2.Close()

	assert.Equal(t, 2, q2.PendingCount())

	changes, err := q2.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	assert.Equal(t, "op1", changes[0].Operation)
	assert.Equal(t, "op2", changes[1].Operation)
}

func TestGetPendingChangesCountNoQueue(t *testing.T) {
	app := newTestApp(t)
	// Disconnect the queue to simulate initialization failure.
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
		app.offlineQueue = nil
	}

	count := app.GetPendingChangesCount()
	assert.Equal(t, 0, count)
}

func TestGetPendingChangesCountWithQueue(t *testing.T) {
	app := newTestApp(t)
	// The test app initializes with a default queue; override with a test one.
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	assert.Equal(t, 0, app.GetPendingChangesCount())

	_ = q.Enqueue("op1", nil)
	_ = q.Enqueue("op2", nil)
	assert.Equal(t, 2, app.GetPendingChangesCount())
}

func TestEnqueueWithNilQueue(t *testing.T) {
	app := newTestApp(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
		app.offlineQueue = nil
	}

	// Should not panic.
	app.enqueue("test_op", map[string]string{"key": "value"})
}

func TestEnqueueAddsToQueue(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	app.enqueue("test_op", map[string]string{"key": "value"})
	assert.Equal(t, 1, q.PendingCount())
}
