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

	assert.Equal(t, 0, q.PendingCount(t.Context()))

	err := q.Enqueue(t.Context(), string(opChangeSet), map[string]string{
		"project_id": "p1", "stream": "main",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, q.PendingCount(t.Context()))

	err = q.Enqueue(t.Context(), string(opDeleteMemoryEntry), map[string]string{
		"entry_id": "e1",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, q.PendingCount(t.Context()))
}

func TestOfflineQueuePeekPendingFIFO(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue(t.Context(), "op1", map[string]string{"key": "first"})
	_ = q.Enqueue(t.Context(), "op2", map[string]string{"key": "second"})
	_ = q.Enqueue(t.Context(), "op3", map[string]string{"key": "third"})

	changes, err := q.PeekPending(t.Context(), 2)
	require.NoError(t, err)
	require.Len(t, changes, 2)

	// FIFO: first enqueued should come first.
	assert.Equal(t, "op1", changes[0].Operation)
	assert.Equal(t, "op2", changes[1].Operation)
	assert.Equal(t, "pending", changes[0].Status)
}

func TestOfflineQueuePeekPendingPayload(t *testing.T) {
	q := newTestQueue(t)

	set := `{"ops":[{"op":"set_content","at":{"doc":"hello.txt","block":"b1","edition":"fr"},"if_match":"r:00000000000000aa","text":"Bonjour le monde"}]}`
	payload := changeSetOp{ProjectID: "p1", Stream: "main", Set: json.RawMessage(set)}
	err := q.Enqueue(t.Context(), string(payload.opKind()), payload)
	require.NoError(t, err)

	changes, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, changes, 1)

	// The payload decodes back into the op, with the change set as it was sent.
	op, err := decodeOp(opKind(changes[0].Operation), changes[0].Payload)
	require.NoError(t, err)
	decoded, ok := op.(changeSetOp)
	require.True(t, ok)
	assert.Equal(t, "p1", decoded.ProjectID)
	assert.Equal(t, "main", decoded.Stream)
	assert.JSONEq(t, set, string(decoded.Set))
}

func TestOfflineQueueMarkCompleted(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue(t.Context(), "op1", nil)
	_ = q.Enqueue(t.Context(), "op2", nil)

	changes, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, changes, 2)

	// Mark first as completed.
	err = q.MarkCompleted(t.Context(), changes[0].ID)
	require.NoError(t, err)

	// Only one pending now.
	assert.Equal(t, 1, q.PendingCount(t.Context()))

	// PeekPending should only return the second.
	remaining, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "op2", remaining[0].Operation)
}

func TestOfflineQueueMarkFailed(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue(t.Context(), "op1", nil)
	changes, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, changes, 1)

	// Mark as failed — it should still be pending (retriable).
	err = q.MarkFailed(t.Context(), changes[0].ID, "network timeout")
	require.NoError(t, err)

	assert.Equal(t, 1, q.PendingCount(t.Context()))

	// Peek again to verify attempts and error.
	changes, err = q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, 1, changes[0].Attempts)
	assert.Equal(t, "network timeout", changes[0].LastError)

	// Mark failed again.
	err = q.MarkFailed(t.Context(), changes[0].ID, "connection refused")
	require.NoError(t, err)

	changes, err = q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	assert.Equal(t, 2, changes[0].Attempts)
	assert.Equal(t, "connection refused", changes[0].LastError)
}

func TestOfflineQueueMarkFailedCapsAttempts(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue(t.Context(), "op1", nil)
	changes, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	id := changes[0].ID

	// Transient failures up to the cap keep the change pending…
	for range maxReplayAttempts - 1 {
		require.NoError(t, q.MarkFailed(t.Context(), id, "network timeout"))
	}
	assert.Equal(t, 1, q.PendingCount(t.Context()))

	// …and the attempt that reaches the cap retires it terminally.
	require.NoError(t, q.MarkFailed(t.Context(), id, "network timeout"))
	assert.Equal(t, 0, q.PendingCount(t.Context()))
	assert.Equal(t, 1, q.FailedCount(t.Context()))

	remaining, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

func TestOfflineQueueMarkFailedPermanent(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue(t.Context(), "op1", nil)
	_ = q.Enqueue(t.Context(), "op2", nil)
	changes, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, changes, 2)

	// A permanent failure leaves the pending set on the first strike.
	require.NoError(t, q.MarkFailedPermanent(t.Context(), changes[0].ID, "review failed (HTTP 422): no translation"))

	assert.Equal(t, 1, q.PendingCount(t.Context()))
	assert.Equal(t, 1, q.FailedCount(t.Context()))

	remaining, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "op2", remaining[0].Operation)
}

func TestOfflineQueuePurgeCompleted(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue(t.Context(), "op1", nil)
	_ = q.Enqueue(t.Context(), "op2", nil)
	_ = q.Enqueue(t.Context(), "op3", nil)

	changes, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)

	// Complete the first two.
	_ = q.MarkCompleted(t.Context(), changes[0].ID)
	_ = q.MarkCompleted(t.Context(), changes[1].ID)

	err = q.PurgeCompleted(t.Context())
	require.NoError(t, err)

	// Only one pending remains.
	assert.Equal(t, 1, q.PendingCount(t.Context()))

	remaining, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "op3", remaining[0].Operation)
}

func TestOfflineQueueClear(t *testing.T) {
	q := newTestQueue(t)

	_ = q.Enqueue(t.Context(), "op1", nil)
	_ = q.Enqueue(t.Context(), "op2", nil)
	_ = q.Enqueue(t.Context(), "op3", nil)
	assert.Equal(t, 3, q.PendingCount(t.Context()))

	err := q.Clear(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 0, q.PendingCount(t.Context()))
}

func TestOfflineQueueEmptyPeek(t *testing.T) {
	q := newTestQueue(t)

	changes, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	assert.Empty(t, changes)
}

func TestOfflineQueuePersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist-queue.db")

	// Create queue and enqueue.
	q1, err := NewOfflineQueue(dbPath)
	require.NoError(t, err)

	_ = q1.Enqueue(t.Context(), "op1", map[string]string{"key": "value"})
	_ = q1.Enqueue(t.Context(), "op2", nil)
	q1.Close()

	// Reopen and verify data persists.
	q2, err := NewOfflineQueue(dbPath)
	require.NoError(t, err)
	defer q2.Close()

	assert.Equal(t, 2, q2.PendingCount(t.Context()))

	changes, err := q2.PeekPending(t.Context(), 10)
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

	_ = q.Enqueue(t.Context(), "op1", nil)
	_ = q.Enqueue(t.Context(), "op2", nil)
	assert.Equal(t, 2, app.GetPendingChangesCount())
}

func TestEnqueueWithNilQueue(t *testing.T) {
	app := newTestApp(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
		app.offlineQueue = nil
	}

	// Should not panic.
	app.enqueue(deleteMemoryEntryOp{EntryID: "e1"})
}

func TestEnqueueAddsToQueue(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	app.enqueue(deleteMemoryEntryOp{EntryID: "e1"})
	assert.Equal(t, 1, q.PendingCount(t.Context()))
}

// The ids of the entries that did not reach the server list the failed and
// dropped ones, oldest first, and follow a dismissal; a pending entry is not
// among them.
func TestOfflineQueueFailedIDs(t *testing.T) {
	q := newTestQueue(t)

	ids, err := q.FailedIDs(t.Context())
	require.NoError(t, err)
	assert.Empty(t, ids)

	for _, op := range []string{"op1", "op2", "op3"} {
		require.NoError(t, q.Enqueue(t.Context(), op, nil))
	}
	changes, err := q.PeekPending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, changes, 3)
	require.NoError(t, q.MarkDropped(t.Context(), changes[2].ID, "queued by an earlier version"))
	require.NoError(t, q.MarkFailedPermanent(t.Context(), changes[0].ID, "HTTP 409"))

	ids, err = q.FailedIDs(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int64{changes[0].ID, changes[2].ID}, ids)

	require.NoError(t, q.Dismiss(t.Context(), changes[0].ID))
	ids, err = q.FailedIDs(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int64{changes[2].ID}, ids)
}
