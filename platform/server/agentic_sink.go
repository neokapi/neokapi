package server

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/redis/go-redis/v9"
)

// initAgenticQueueSink sets up the event-to-queue adapter when
// BOWRAIN_AGENTIC_EVENTS is enabled. It prefers Service Bus (durable, KEDA-
// compatible) and falls back to Redis pub/sub for local development.
func (s *Server) initAgenticQueueSink(cfg ServerConfig) {
	if !cfg.AgenticEvents {
		return
	}

	var publisher event.QueuePublisher
	var backend string
	var prefix string

	switch {
	case cfg.ServiceBusConnection != "":
		pub, err := newServiceBusPublisher(cfg.ServiceBusConnection)
		if err != nil {
			log.Printf("WARNING: agentic events: failed to connect to Service Bus: %v", err)
			return
		}
		publisher = pub
		backend = "servicebus"

	case cfg.RedisURL != "":
		pub, err := newRedisPublisher(cfg.RedisURL, cfg.RedisPassword)
		if err != nil {
			log.Printf("WARNING: agentic events: failed to connect to Redis: %v", err)
			return
		}
		publisher = pub
		backend = "redis"
		prefix = "agentic:"

	default:
		log.Printf("WARNING: agentic events enabled but no Redis or Service Bus configured; skipping")
		return
	}

	s.AgenticQueueSink = event.NewQueueSink(s.EventBus, publisher, event.QueueSinkConfig{
		Enabled:       true,
		ChannelPrefix: prefix,
	})

	log.Printf("Agentic event queue sink active (backend=%s)", backend)
}

// newRedisPublisher creates a QueuePublisher backed by Redis pub/sub.
func newRedisPublisher(redisURL, redisPassword string) (event.QueuePublisher, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	if redisPassword != "" {
		opts.Password = redisPassword
	}
	client := redis.NewClient(opts)
	return event.NewRedisQueuePublisher(func(ctx context.Context, channel string, message interface{}) error {
		return client.Publish(ctx, channel, message).Err()
	}), nil
}

// newServiceBusPublisher creates a QueuePublisher backed by Azure Service Bus.
// It lazily creates senders per queue name.
func newServiceBusPublisher(connStr string) (event.QueuePublisher, error) {
	client, err := azservicebus.NewClientFromConnectionString(connStr, nil)
	if err != nil {
		return nil, fmt.Errorf("create Service Bus client: %w", err)
	}

	var mu sync.Mutex
	senders := make(map[string]*azservicebus.Sender)

	return event.NewServiceBusQueuePublisher(func(ctx context.Context, queue string, body []byte) error {
		mu.Lock()
		sender, ok := senders[queue]
		if !ok {
			var err error
			sender, err = client.NewSender(queue, nil)
			if err != nil {
				mu.Unlock()
				return fmt.Errorf("create sender for %q: %w", queue, err)
			}
			senders[queue] = sender
		}
		mu.Unlock()

		return sender.SendMessage(ctx, &azservicebus.Message{Body: body}, nil)
	}), nil
}
