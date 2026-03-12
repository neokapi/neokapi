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
	ctx := context.Background()

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
	ctx := context.Background()

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

func TestChannelQueue_DequeueContextCancelled(t *testing.T) {
	q := NewChannelQueue(10)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, _, err := q.Dequeue(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestChannelQueue_Close(t *testing.T) {
	q := NewChannelQueue(10)
	require.NoError(t, q.Close())

	err := q.Enqueue(context.Background(), "x")
	assert.Error(t, err)
}
