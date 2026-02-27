package jobs

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestNATS starts an embedded NATS server with JetStream enabled
// on a random port and returns the client URL. The server is stopped
// automatically when the test finishes.
func startTestNATS(t *testing.T) string {
	t.Helper()
	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1, // random available port
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	srv, err := natsserver.NewServer(opts)
	require.NoError(t, err)

	srv.Start()
	t.Cleanup(srv.Shutdown)

	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready within 5s")
	}

	return srv.ClientURL()
}

func TestNATSQueue_EnqueueDequeue(t *testing.T) {
	url := startTestNATS(t)
	q, err := NewNATSQueue(url)
	require.NoError(t, err)
	defer q.Close()

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

func TestNATSQueue_Nack(t *testing.T) {
	url := startTestNATS(t)
	q, err := NewNATSQueue(url)
	require.NoError(t, err)
	defer q.Close()

	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "job-retry"))

	id, _, nack, err := q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "job-retry", id)
	nack() // return to queue for redelivery

	// Should be available again after nack.
	id, ack, _, err := q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "job-retry", id)
	ack()
}

func TestNATSQueue_DequeueContextCancelled(t *testing.T) {
	url := startTestNATS(t)
	q, err := NewNATSQueue(url)
	require.NoError(t, err)
	defer q.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, _, err = q.Dequeue(ctx)
	assert.Error(t, err)
}
