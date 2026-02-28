package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	natsStreamName = "JOBS"
	natsSubject    = "JOBS.translate"
	natsConsumer   = "worker"
	natsMaxDeliver = 3
	natsAckWait    = 5 * time.Minute
	natsFetchWait  = 5 * time.Second
)

// NATSQueue implements Queue using NATS JetStream.
type NATSQueue struct {
	conn     *nats.Conn
	js       jetstream.JetStream
	consumer jetstream.Consumer
}

// NewNATSQueue connects to a NATS server and ensures the JetStream stream
// and consumer exist. The stream uses WorkQueuePolicy so messages are removed
// after acknowledgement, matching Azure Service Bus semantics.
func NewNATSQueue(url string) (*NATSQueue, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS at %s: %w", url, err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}

	ctx := context.Background()

	// CreateOrUpdate is idempotent — safe to call on every startup.
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      natsStreamName,
		Subjects:  []string{natsSubject},
		Retention: jetstream.WorkQueuePolicy,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create/update stream %q: %w", natsStreamName, err)
	}

	consumer, err := js.CreateOrUpdateConsumer(ctx, natsStreamName, jetstream.ConsumerConfig{
		Durable:    natsConsumer,
		AckPolicy:  jetstream.AckExplicitPolicy,
		MaxDeliver: natsMaxDeliver,
		AckWait:    natsAckWait,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create/update consumer %q: %w", natsConsumer, err)
	}

	return &NATSQueue{
		conn:     nc,
		js:       js,
		consumer: consumer,
	}, nil
}

func (q *NATSQueue) Enqueue(ctx context.Context, jobID string) error {
	if _, err := q.js.Publish(ctx, natsSubject, []byte(jobID)); err != nil {
		return fmt.Errorf("enqueue job %s: %w", jobID, err)
	}
	return nil
}

func (q *NATSQueue) Dequeue(ctx context.Context) (string, func(), func(), error) {
	batch, err := q.consumer.Fetch(1, jetstream.FetchMaxWait(natsFetchWait))
	if err != nil {
		return "", nil, nil, fmt.Errorf("fetch message: %w", err)
	}

	for msg := range batch.Messages() {
		jobID := string(msg.Data())
		ack := func() { _ = msg.Ack() }
		nack := func() { _ = msg.Nak() }
		return jobID, ack, nack, nil
	}

	// No messages within the fetch window — check context or return timeout.
	if err := batch.Error(); err != nil {
		return "", nil, nil, fmt.Errorf("fetch batch: %w", err)
	}
	if ctx.Err() != nil {
		return "", nil, nil, ctx.Err()
	}
	return "", nil, nil, fmt.Errorf("no messages available")
}

func (q *NATSQueue) Close() error {
	q.conn.Close()
	return nil
}
