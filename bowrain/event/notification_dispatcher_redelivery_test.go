package event

import (
	"context"
	"sync"
	"testing"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingMailer records the immediate emails a dispatcher sends.
type countingMailer struct {
	mu   sync.Mutex
	sent []string
}

func (m *countingMailer) SendImmediate(_ context.Context, userID string, _ *bstore.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, userID)
	return nil
}

func (m *countingMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

// TestNotificationDispatcher_RedeliveryTellsNobodyTwice is the deploy-window
// property. The consumer group is at-least-once, so a SIGTERM mid-batch leaves
// entries to be reclaimed and re-dispatched. Without a key on the event, every
// project member got a second inbox row and a second immediate email each time.
func TestNotificationDispatcher_RedeliveryTellsNobodyTwice(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	notifStore := newTestNotifStore(t)
	sender := &mockSender{}
	mailer := &countingMailer{}
	targetFn := func(context.Context, string, string) ([]string, error) {
		return []string{"user-1", "user-2"}, nil
	}

	d := NewNotificationDispatcher(bus, notifStore, nil, sender, targetFn)
	defer d.Close()
	d.SetMailer(mailer)

	// A high-priority event, so the email path runs too.
	ev := platev.Event{
		ID:        "evt-gate-1",
		Type:      platev.EventQualityGateFail,
		ProjectID: "proj-1",
		Data:      map[string]string{"workspace_slug": "ws-1"},
	}

	// Dispatched directly rather than through the bus: the redelivery under
	// test is a second call with the same event, and the assertion is about
	// what the second call does, not about delivery timing.
	for range 4 {
		require.NoError(t, d.handleEvent(ev))
	}

	for _, user := range []string{"user-1", "user-2"} {
		notifs, err := notifStore.List(t.Context(), user, 10, false)
		require.NoError(t, err)
		assert.Len(t, notifs, 1, "%s has one inbox row for one event", user)
	}
	assert.Equal(t, 2, sender.count(), "one push per person, not per delivery")
	assert.Equal(t, 2, mailer.count(), "one email per person, not per delivery")

	// A different event about the same project still reaches everyone.
	next := ev
	next.ID = "evt-gate-2"
	require.NoError(t, d.handleEvent(next))
	assert.Equal(t, 4, sender.count())
	assert.Equal(t, 4, mailer.count())
}
