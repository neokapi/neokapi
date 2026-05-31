package event

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
)

const (
	natsEventStream  = "EVENTS"
	natsEventSubject = "EVENTS"   // publish to EVENTS.<type>
	natsEventFilter  = "EVENTS.>" // subscribe to all events
	natsMaxDeliver   = 5
	natsEventAckWait = 30 * time.Second

	// instanceIDEnv overrides the per-instance identity used to name
	// non-competing ("fan-out") durable consumers. In a clustered deployment
	// each pod/replica must carry a stable, distinct identity so that
	// SubscribeAll consumers (a) do not compete with each other across pods and
	// (b) survive a restart of the *same* pod, replaying any JetStream messages
	// that were pending when the pod went down. Set this to a value that is
	// stable across restarts of a given replica (e.g. a Kubernetes StatefulSet
	// pod name, or `metadata.name` projected into the env).
	instanceIDEnv = "BOWRAIN_INSTANCE_ID"
)

// NATSEventBus implements EventBus using NATS JetStream.
// Publish sends to EVENTS.<type>. SubscribeGroup creates a durable
// consumer per group with competing consumer semantics.
type NATSEventBus struct {
	nc *nats.Conn
	js jetstream.JetStream

	// instanceID is a stable-across-restarts identity for this process, used to
	// name non-competing fan-out durable consumers (SubscribeAll). See
	// instanceIDEnv and resolveInstanceID.
	instanceID string

	// fanoutSeq assigns each SubscribeAll call a deterministic ordinal within
	// the process. Because the server registers its SubscribeAll subscribers in
	// the same order on every boot (ChangeRelay, GraphSyncer, gRPC watchers,
	// …), the Nth call gets the same suffix across restarts, yielding a stable
	// durable name per (instance, subscriber).
	fanoutSeq atomic.Uint64

	mu        sync.Mutex
	consumers map[string]*natsConsumer
	closed    bool
}

type natsConsumer struct {
	sub      *platev.Subscription
	cancel   context.CancelFunc
	done     chan struct{}
	durable  string // JetStream durable name backing this subscription
	deleteOn bool   // delete the JetStream consumer when this subscription ends
}

// NewNATSEventBus creates an event bus backed by NATS JetStream.
func NewNATSEventBus(url string) (*NATSEventBus, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, err
	}

	// Create or update the events stream.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      natsEventStream,
		Subjects:  []string{natsEventFilter},
		Retention: jetstream.InterestPolicy, // Keep messages while consumers have interest
		MaxAge:    7 * 24 * time.Hour,
	})
	if err != nil {
		nc.Close()
		return nil, err
	}

	return &NATSEventBus{
		nc:         nc,
		js:         js,
		instanceID: resolveInstanceID(),
		consumers:  make(map[string]*natsConsumer),
	}, nil
}

// resolveInstanceID derives a stable-across-restarts identity for this process.
// Preference order:
//
//  1. BOWRAIN_INSTANCE_ID — explicit, operator-controlled (e.g. a StatefulSet
//     pod name). Use this in clustered deployments.
//  2. os.Hostname() — stable for a given container/pod/VM across process
//     restarts, and distinct between replicas in most orchestrators.
//  3. a random fallback — only when neither is available. This is *not* stable
//     across restarts, so pending fan-out messages would not replay; we log so
//     the degraded behaviour is visible.
//
// The result is sanitised to the alphanumeric/`-`/`_` set that JetStream
// durable names allow.
func resolveInstanceID() string {
	if v := strings.TrimSpace(os.Getenv(instanceIDEnv)); v != "" {
		return sanitizeDurable(v)
	}
	if host, err := os.Hostname(); err == nil {
		if h := strings.TrimSpace(host); h != "" {
			return sanitizeDurable(h)
		}
	}
	slog.Warn("nats-event-bus: no stable instance identity (BOWRAIN_INSTANCE_ID unset, hostname unavailable); " +
		"fan-out consumers will use a random name and will not replay pending messages across restarts")
	return "anon-" + id.New()[:8]
}

// sanitizeDurable maps an arbitrary identity string onto the character set that
// JetStream permits in durable/consumer names (alphanumeric, '-', '_'). Any
// other byte (including the '.', '*', '>', whitespace that NATS reserves) is
// replaced with '_'. The mapping is deterministic, so a given identity always
// yields the same durable name across restarts.
func sanitizeDurable(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "x"
	}
	return out
}

// Publish sends an event to EVENTS.<type>.
func (b *NATSEventBus) Publish(ev platev.Event) {
	if ev.ID == "" {
		ev.ID = id.New()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(ev)
	if err != nil {
		slog.Info("nats-event-bus: marshal error", "error", err)
		return
	}

	subject := natsEventSubject + "." + string(ev.Type)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := b.js.Publish(ctx, subject, data); err != nil {
		slog.Info("nats-event-bus: publish error", "error", err)
	}
}

// Subscribe creates a consumer filtered by a single event type with the same
// non-competing ("fan-out") semantics as SubscribeAll: every instance (and, for
// the gRPC watch handlers, every concurrent watcher) receives its own copy of
// matching events. The durable name is therefore scoped to this instance and a
// per-process call ordinal — e.g. "type-block.updated-<instanceID>-1" — so it is
// distinct per instance/watcher yet stable across restarts of the same process,
// letting JetStream replay messages that were pending at restart.
func (b *NATSEventBus) Subscribe(eventType platev.EventType, handler platev.EventHandler) *platev.Subscription {
	seq := b.fanoutSeq.Add(1)
	durable := fmt.Sprintf("type-%s-%s-%d", sanitizeDurable(string(eventType)), b.instanceID, seq)
	// Per-connection gRPC watchers come and go with a fresh ordinal each time,
	// so their durables must be reclaimed on Unsubscribe — otherwise abandoned
	// consumers accumulate in JetStream and (under InterestPolicy retention)
	// keep matching messages alive forever. A hard restart (no Unsubscribe) still
	// leaves the durable behind, which JetStream reclaims once interest lapses.
	return b.subscribeInternal(durable, natsEventSubject+"."+string(eventType), handler, true)
}

// SubscribeAll subscribes to all events with non-competing ("fan-out")
// semantics: every server instance receives a copy of every event (used by
// ChangeRelay, GraphSyncer, and the gRPC watch handlers).
//
// The consumer is durable and named deterministically from this instance's
// identity plus a per-process call ordinal — e.g. "all-<instanceID>-1". This
// gives each instance its *own* consumer (so instances don't compete) while
// keeping the name stable across restarts of the same instance, so JetStream
// retains and replays any messages that were pending when the process went
// down. (The previous random "all-<rand>" name created a fresh consumer on
// every boot, abandoning pending messages — see audit finding #46.)
//
// On graceful Unsubscribe/Close the JetStream consumer is deleted (no orphan
// accumulation); on a hard crash it is left intact so that the deterministic
// name is re-bound on restart and pending messages are replayed.
func (b *NATSEventBus) SubscribeAll(handler platev.EventHandler) *platev.Subscription {
	seq := b.fanoutSeq.Add(1)
	durable := fmt.Sprintf("all-%s-%d", b.instanceID, seq)
	return b.subscribeInternal(durable, natsEventFilter, handler, true)
}

// SubscribeGroup creates a durable consumer with competing consumer semantics.
// The group name is shared across instances, so the durable is shared and must
// persist beyond any single subscriber's lifetime — it is never deleted on
// Unsubscribe (other instances in the group rely on it).
func (b *NATSEventBus) SubscribeGroup(group string, handler platev.EventHandler) *platev.Subscription {
	return b.subscribeInternal(sanitizeDurable(group), natsEventFilter, handler, false)
}

func (b *NATSEventBus) subscribeInternal(consumerName, filter string, handler platev.EventHandler, deleteOnUnsubscribe bool) *platev.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	sub := &platev.Subscription{
		ID:      id.New(),
		Group:   consumerName,
		Handler: handler,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create or update durable consumer.
	consumer, err := b.js.CreateOrUpdateConsumer(ctx, natsEventStream, jetstream.ConsumerConfig{
		Durable:       consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: filter,
		MaxDeliver:    natsMaxDeliver,
		AckWait:       natsEventAckWait,
	})
	if err != nil {
		slog.Info("nats-event-bus: failed to create consumer", "id", consumerName, "error", err)
		cancel()
		return sub
	}

	nc := &natsConsumer{
		sub:      sub,
		cancel:   cancel,
		done:     make(chan struct{}),
		durable:  consumerName,
		deleteOn: deleteOnUnsubscribe,
	}
	b.consumers[sub.ID] = nc

	go b.consumeLoop(ctx, consumer, nc)

	return sub
}

func (b *NATSEventBus) consumeLoop(ctx context.Context, consumer jetstream.Consumer, nc *natsConsumer) {
	defer close(nc.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		for msg := range batch.Messages() {
			var ev platev.Event
			if err := json.Unmarshal(msg.Data(), &ev); err != nil {
				slog.Info("nats-event-bus: unmarshal error", "error", err)
				_ = msg.Ack()
				continue
			}

			nc.sub.Handler(ev)
			_ = msg.Ack()
		}
	}
}

// Unsubscribe stops a consumer. For subscriptions flagged deleteOn (the
// fan-out paths: Subscribe/SubscribeAll), the backing JetStream durable is also
// removed so that orphaned consumers don't pile up. Shared group durables
// (SubscribeGroup) are left intact for the other instances in the group.
func (b *NATSEventBus) Unsubscribe(sub *platev.Subscription) {
	b.mu.Lock()
	nc, ok := b.consumers[sub.ID]
	if ok {
		delete(b.consumers, sub.ID)
	}
	b.mu.Unlock()

	if ok {
		nc.cancel()
		<-nc.done
		b.maybeDeleteConsumer(nc)
	}
}

// maybeDeleteConsumer removes the JetStream durable backing nc when it was
// created with deleteOn semantics. Errors are logged and swallowed: a missing
// consumer (already gone) is benign, and a transient delete failure must not
// block teardown.
func (b *NATSEventBus) maybeDeleteConsumer(nc *natsConsumer) {
	if !nc.deleteOn || nc.durable == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.js.DeleteConsumer(ctx, natsEventStream, nc.durable); err != nil {
		slog.Info("nats-event-bus: failed to delete consumer", "durable", nc.durable, "error", err)
	}
}

// Close shuts down all consumers and the NATS connection.
func (b *NATSEventBus) Close() {
	b.mu.Lock()
	b.closed = true
	consumers := make([]*natsConsumer, 0, len(b.consumers))
	for _, nc := range b.consumers {
		consumers = append(consumers, nc)
	}
	b.consumers = make(map[string]*natsConsumer)
	b.mu.Unlock()

	for _, nc := range consumers {
		nc.cancel()
		<-nc.done
		b.maybeDeleteConsumer(nc)
	}
	b.nc.Close()
}
