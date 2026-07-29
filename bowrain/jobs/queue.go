package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Queue is an abstraction for enqueuing and dequeuing translation job IDs.
type Queue interface {
	// Enqueue adds a job ID to the queue.
	Enqueue(ctx context.Context, jobID string) error

	// Dequeue blocks until a job ID is available. The returned ack function
	// marks the message as successfully processed; nack returns it to the
	// queue for retry.
	Dequeue(ctx context.Context) (jobID string, ack func(), nack func(), err error)

	// Healthy reports whether the queue connection is alive.
	Healthy() bool

	// Close releases queue resources.
	Close() error
}

// DelayedQueue is the optional half of [Queue]: a broker that can hold a
// message invisible for a while before delivering it. Deferring a job onto a
// down dependency needs exactly this — redeliver when the circuit is due to
// probe, not immediately.
//
// It is a separate interface rather than a Queue method so a backend that
// cannot delay (or a test double) stays a valid Queue; [enqueueAfter] falls
// back to an immediate Enqueue for those.
type DelayedQueue interface {
	// EnqueueAfter adds a job ID that becomes visible to consumers only after
	// delay. A non-positive delay is equivalent to Enqueue.
	EnqueueAfter(ctx context.Context, jobID string, delay time.Duration) error
}

// enqueueAfter defers a job when the queue supports it, and enqueues it
// immediately otherwise. The fallback is safe rather than ideal: the job may be
// picked up before the dependency recovers, find the circuit still open, and be
// deferred again — correct, just less patient.
func enqueueAfter(ctx context.Context, q Queue, jobID string, delay time.Duration) error {
	if dq, ok := q.(DelayedQueue); ok && delay > 0 {
		return dq.EnqueueAfter(ctx, jobID, delay)
	}
	return q.Enqueue(ctx, jobID)
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
		return errors.New("queue is closed")
	}
	select {
	case q.ch <- jobID:
		return nil
	default:
		return errors.New("queue is full")
	}
}

// EnqueueAfter makes ChannelQueue a [DelayedQueue] by parking the re-enqueue on
// a timer. The timer goroutine is fire-and-forget by necessity — the caller was
// told the deferral was accepted and has already moved on — but a deferred job
// is one the system promised to run again, so the send waits for room rather
// than abandoning the job on a full buffer. That distinction matters: a full
// buffer is precisely the load that produces retries, so dropping there loses
// the jobs most in need of running. The delay is honored against ctx so a
// shutting-down worker does not leave timers behind, and the two remaining ways
// the send can fail — a closed queue, a cancelled context — are logged.
func (q *ChannelQueue) EnqueueAfter(ctx context.Context, jobID string, delay time.Duration) error {
	if delay <= 0 {
		return q.Enqueue(ctx, jobID)
	}
	q.mu.Lock()
	closed := q.closed
	q.mu.Unlock()
	if closed {
		return errors.New("queue is closed")
	}

	go func() {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := q.enqueueWaiting(ctx, jobID); err != nil {
			slog.Error("deferred job was not re-queued", "job_id", jobID, "error", err)
		}
	}()
	return nil
}

// enqueueWaiting sends jobID once the buffer has room, giving up only when ctx
// ends or the queue closes.
//
// It polls instead of parking on the send because Close closes the channel: a
// send parked while holding q.mu would deadlock Close, and one parked without it
// would panic if Close won the race. Keeping every send attempt non-blocking
// under the lock avoids both. The cost is up to one poll interval of latency on
// a full buffer — an already-degraded state, in the queue that exists for
// single-process deployments.
func (q *ChannelQueue) enqueueWaiting(ctx context.Context, jobID string) error {
	const pollInterval = 25 * time.Millisecond

	tick := time.NewTicker(pollInterval)
	defer tick.Stop()

	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return errors.New("queue is closed")
		}
		select {
		case q.ch <- jobID:
			q.mu.Unlock()
			return nil
		default:
		}
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func (q *ChannelQueue) Dequeue(ctx context.Context) (string, func(), func(), error) {
	select {
	case <-ctx.Done():
		return "", nil, nil, ctx.Err()
	case jobID, ok := <-q.ch:
		if !ok {
			return "", nil, nil, errors.New("queue is closed")
		}
		ack := func() {} // channel dequeue is already consuming
		nack := func() {
			// Returning the job to the queue is the whole point of a nack, so
			// wait for room rather than dropping it. enqueueWaiting also takes
			// q.mu, which a bare send did not: sending on the channel after
			// Close had closed it would panic the worker.
			if err := q.enqueueWaiting(ctx, jobID); err != nil {
				slog.Error("nacked job was not re-queued", "job_id", jobID, "error", err)
			}
		}
		return jobID, ack, nack, nil
	}
}

func (q *ChannelQueue) Healthy() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return !q.closed
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
