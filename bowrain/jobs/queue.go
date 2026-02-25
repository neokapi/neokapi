package jobs

import (
	"context"
	"fmt"
	"sync"
)

// Queue is an abstraction for enqueuing and dequeuing translation job IDs.
type Queue interface {
	// Enqueue adds a job ID to the queue.
	Enqueue(ctx context.Context, jobID string) error

	// Dequeue blocks until a job ID is available. The returned ack function
	// marks the message as successfully processed; nack returns it to the
	// queue for retry.
	Dequeue(ctx context.Context) (jobID string, ack func(), nack func(), err error)

	// Close releases queue resources.
	Close() error
}

// ChannelQueue is an in-memory Queue backed by a Go channel.
// Suitable for local development and testing.
type ChannelQueue struct {
	ch     chan string
	closed bool
	mu     sync.Mutex
}

// NewChannelQueue creates a ChannelQueue with the given buffer size.
func NewChannelQueue(bufferSize int) *ChannelQueue {
	if bufferSize <= 0 {
		bufferSize = 64
	}
	return &ChannelQueue{ch: make(chan string, bufferSize)}
}

func (q *ChannelQueue) Enqueue(_ context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return fmt.Errorf("queue is closed")
	}
	select {
	case q.ch <- jobID:
		return nil
	default:
		return fmt.Errorf("queue is full")
	}
}

func (q *ChannelQueue) Dequeue(ctx context.Context) (string, func(), func(), error) {
	select {
	case <-ctx.Done():
		return "", nil, nil, ctx.Err()
	case jobID, ok := <-q.ch:
		if !ok {
			return "", nil, nil, fmt.Errorf("queue is closed")
		}
		ack := func() {} // channel dequeue is already consuming
		nack := func() {
			// Re-enqueue for retry (best-effort; may block if full).
			select {
			case q.ch <- jobID:
			default:
			}
		}
		return jobID, ack, nack, nil
	}
}

func (q *ChannelQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.ch)
	}
	return nil
}
