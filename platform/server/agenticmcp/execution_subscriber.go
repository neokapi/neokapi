package agenticmcp

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
)

const agenticEventsChannel = "agentic:events"

// ExecutionSubscriber subscribes to Redis pub/sub and persists agentic events
// to the PostgresExecutionStore.
type ExecutionSubscriber struct {
	client *redis.Client
	store  *PostgresExecutionStore
	cancel context.CancelFunc
}

// NewExecutionSubscriber creates a subscriber that connects to Redis and
// writes events to the execution store.
func NewExecutionSubscriber(redisURL, redisPassword string, store *PostgresExecutionStore) (*ExecutionSubscriber, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	if redisPassword != "" {
		opts.Password = redisPassword
	}
	client := redis.NewClient(opts)

	return &ExecutionSubscriber{
		client: client,
		store:  store,
	}, nil
}

// Start begins subscribing to the agentic events channel in a goroutine.
func (s *ExecutionSubscriber) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	go func() {
		pubsub := s.client.Subscribe(ctx, agenticEventsChannel)
		defer pubsub.Close()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				s.handleMessage(ctx, msg.Payload)
			}
		}
	}()
}

// Close stops the subscriber and closes the Redis connection.
func (s *ExecutionSubscriber) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.client.Close()
}

func (s *ExecutionSubscriber) handleMessage(ctx context.Context, payload string) {
	var ev AgenticEvent
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		log.Printf("agentic subscriber: invalid event JSON: %v", err)
		return
	}

	// Insert into the event log.
	if err := s.store.InsertEvent(ctx, &ev); err != nil {
		log.Printf("agentic subscriber: insert event: %v", err)
	}

	// Upsert execution lifecycle (only for exec.started/completed/failed).
	if err := s.store.UpsertExecution(ctx, &ev); err != nil {
		log.Printf("agentic subscriber: upsert execution: %v", err)
	}
}
