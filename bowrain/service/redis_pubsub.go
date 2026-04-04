package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// SSEEvent is a serializable SSE event for Redis pub/sub transport.
type SSEEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// AgentPubSub provides Redis pub/sub for relaying agent SSE events
// between the worker (publisher) and the API server (subscriber).
type AgentPubSub struct {
	client *redis.Client
}

// NewAgentPubSub creates a Redis pub/sub client.
func NewAgentPubSub(client *redis.Client) *AgentPubSub {
	return &AgentPubSub{client: client}
}

// Publish sends an SSE event to the Redis channel for a conversation.
func (p *AgentPubSub) Publish(ctx context.Context, conversationID string, event SSEEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal SSE event: %w", err)
	}
	return p.client.Publish(ctx, channelName(conversationID), data).Err()
}

// Subscribe listens for SSE events on the Redis channel for a conversation.
// Returns a channel of events and a cancel function. The event channel is
// closed when the context is cancelled or cancel is called.
func (p *AgentPubSub) Subscribe(ctx context.Context, conversationID string) (<-chan SSEEvent, func()) {
	channel := channelName(conversationID)
	sub := p.client.Subscribe(ctx, channel)

	// Wait for subscription confirmation.
	if _, err := sub.Receive(ctx); err != nil {
		slog.Warn("Redis subscribe failed", "channel", channel, "error", err)
	} else {
		slog.Info("Redis subscribed", "channel", channel)
	}

	ch := make(chan SSEEvent, 64)
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(ch)
		defer sub.Close()

		msgCh := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				var evt SSEEvent
				if json.Unmarshal([]byte(msg.Payload), &evt) == nil {
					select {
					case ch <- evt:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch, cancel
}

func channelName(conversationID string) string {
	return "bravo:sse:" + conversationID
}
