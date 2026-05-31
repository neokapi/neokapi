package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// countingEventBus is a minimal platev.EventBus used to assert that every
// subscription created by Subscribe is torn down when the stream ends. It
// records each *Subscription it hands out and removes it on Unsubscribe so a
// leaked handle shows up as a leftover entry.
type countingEventBus struct {
	mu       sync.Mutex
	nextID   int
	active   map[string]*platev.Subscription
	subCalls int
	allCalls int
	unsubbed []string
}

func newCountingEventBus() *countingEventBus {
	return &countingEventBus{active: make(map[string]*platev.Subscription)}
}

func (b *countingEventBus) newSub(t platev.EventType, h platev.EventHandler) *platev.Subscription {
	b.nextID++
	sub := &platev.Subscription{
		ID:        fmt.Sprintf("sub-%d", b.nextID),
		EventType: t,
		Handler:   h,
	}
	b.active[sub.ID] = sub
	return sub
}

func (b *countingEventBus) Publish(_ platev.Event) {}

func (b *countingEventBus) Subscribe(eventType platev.EventType, handler platev.EventHandler) *platev.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subCalls++
	return b.newSub(eventType, handler)
}

func (b *countingEventBus) SubscribeAll(handler platev.EventHandler) *platev.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.allCalls++
	return b.newSub("", handler)
}

func (b *countingEventBus) SubscribeGroup(_ string, handler platev.EventHandler) *platev.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.newSub("", handler)
}

func (b *countingEventBus) Unsubscribe(sub *platev.Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub == nil {
		return
	}
	if _, ok := b.active[sub.ID]; ok {
		delete(b.active, sub.ID)
		b.unsubbed = append(b.unsubbed, sub.ID)
	}
}

func (b *countingEventBus) Close() {}

func (b *countingEventBus) activeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.active)
}

// fakeSubscribeStream implements pb.NeokapiService_SubscribeServer
// (grpc.ServerStreamingServer[pb.EventResponse]) with a controllable context.
type fakeSubscribeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeSubscribeStream) Context() context.Context { return s.ctx }

func (s *fakeSubscribeStream) Send(*pb.EventResponse) error { return nil }

// TestSubscribe_UnsubscribesAllOnDisconnect verifies Finding #9: every
// subscription created for the requested event types is unsubscribed when the
// client disconnects — not just the first one.
func TestSubscribe_UnsubscribesAllOnDisconnect(t *testing.T) {
	tests := []struct {
		name         string
		eventTypes   []string
		wantSubCalls int
		wantAllCalls int
		wantUnsubbed int
	}{
		{
			name:         "single type",
			eventTypes:   []string{"block.updated"},
			wantSubCalls: 1,
			wantUnsubbed: 1,
		},
		{
			name:         "multiple types",
			eventTypes:   []string{"block.updated", "item.created", "flow.completed"},
			wantSubCalls: 3,
			wantUnsubbed: 3,
		},
		{
			name:         "no types means subscribe all",
			eventTypes:   nil,
			wantAllCalls: 1,
			wantUnsubbed: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bus := newCountingEventBus()
			g := &GRPCServer{srv: &Server{EventBus: bus}}

			ctx, cancel := context.WithCancel(context.Background())
			stream := &fakeSubscribeStream{ctx: ctx}

			done := make(chan error, 1)
			go func() {
				done <- g.Subscribe(&pb.SubscribeRequest{EventTypes: tc.eventTypes}, stream)
			}()

			// Disconnect the client.
			cancel()

			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				t.Fatal("Subscribe did not return after context cancellation")
			}

			assert.Equal(t, tc.wantSubCalls, bus.subCalls, "Subscribe call count")
			assert.Equal(t, tc.wantAllCalls, bus.allCalls, "SubscribeAll call count")
			assert.Len(t, bus.unsubbed, tc.wantUnsubbed, "every subscription must be unsubscribed")
			assert.Equal(t, 0, bus.activeCount(), "no subscriptions may leak after disconnect")
		})
	}
}

// TestSubscribe_NoEventBus verifies the guard when no bus is configured.
func TestSubscribe_NoEventBus(t *testing.T) {
	g := &GRPCServer{srv: &Server{}}
	err := g.Subscribe(&pb.SubscribeRequest{}, &fakeSubscribeStream{ctx: context.Background()})
	require.Error(t, err)
}
