package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelQueue_EnqueueDequeue(t *testing.T) {
	q := NewChannelQueue(10)
	ctx := t.Context()

	require.NoError(t, q.Enqueue(ctx, "job-1"))
	require.NoError(t, q.Enqueue(ctx, "job-2"))

	id, ack, _, err := q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "job-1", id)
	ack()

	id, ack, _, err = q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "job-2", id)
	ack()
}

func TestChannelQueue_Nack(t *testing.T) {
	q := NewChannelQueue(10)
	ctx := t.Context()

	require.NoError(t, q.Enqueue(ctx, "job-retry"))

	id, _, nack, err := q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "job-retry", id)
	nack() // re-enqueue

	// Should be available again.
	id, ack, _, err := q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "job-retry", id)
	ack()
}

// Returning the job is the whole purpose of a nack, so a full buffer must not
// be where it quietly stops being a retry.
func TestChannelQueue_NackWaitsForRoom(t *testing.T) {
	q := NewChannelQueue(1)
	defer q.Close()
	ctx := t.Context()

	require.NoError(t, q.Enqueue(ctx, "job-retry"))

	id, _, nack, err := q.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, "job-retry", id)

	// Refill the single slot before nacking, so the nack finds no room.
	require.NoError(t, q.Enqueue(ctx, "job-occupant"))

	done := make(chan struct{})
	go func() {
		defer close(done)
		nack()
	}()

	// Let the nack reach the full buffer before anything drains it — otherwise
	// it may win the race, find room, and never exercise the full-buffer path.
	time.Sleep(150 * time.Millisecond)

	// Drain the occupant, making room for the nacked job.
	select {
	case got := <-q.ch:
		require.Equal(t, "job-occupant", got)
	case <-time.After(time.Second):
		t.Fatal("occupant was never delivered")
	}

	select {
	case got := <-q.ch:
		assert.Equal(t, "job-retry", got)
	case <-time.After(2 * time.Second):
		t.Fatal("nacked job was dropped when the buffer was full")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("nack never returned")
	}
}

// Close closes the channel, so a nack that sent on it directly would panic the
// worker rather than report that the queue had gone away.
func TestChannelQueue_NackAfterCloseDoesNotPanic(t *testing.T) {
	q := NewChannelQueue(1)
	ctx := t.Context()

	require.NoError(t, q.Enqueue(ctx, "job-retry"))
	_, _, nack, err := q.Dequeue(ctx)
	require.NoError(t, err)

	require.NoError(t, q.Close())
	assert.NotPanics(t, nack)
}

func TestChannelQueue_DequeueContextCancelled(t *testing.T) {
	q := NewChannelQueue(10)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, _, _, err := q.Dequeue(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestChannelQueue_Close(t *testing.T) {
	q := NewChannelQueue(10)
	require.NoError(t, q.Close())

	err := q.Enqueue(t.Context(), "x")
	assert.Error(t, err)
}
