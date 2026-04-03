package agenticmcp

import (
	"sync"
)

// EventHub broadcasts agentic events to connected WebSocket clients.
// The subscriber pushes events after persisting; the hub fans out to all
// connected dashboard sessions.
type EventHub struct {
	mu      sync.RWMutex
	clients map[*EventClient]struct{}
}

// EventClient represents a connected dashboard WebSocket session.
type EventClient struct {
	// C receives events broadcast by the hub. Buffered to avoid blocking
	// the hub on slow clients.
	C chan AgenticEvent

	// Filter optionally restricts which events this client receives.
	WorkspaceSlug string // empty = all workspaces
}

// NewEventHub creates a new broadcast hub.
func NewEventHub() *EventHub {
	return &EventHub{
		clients: make(map[*EventClient]struct{}),
	}
}

// Subscribe registers a client to receive events.
func (h *EventHub) Subscribe(client *EventClient) {
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
}

// Unsubscribe removes a client and closes its channel.
func (h *EventHub) Unsubscribe(client *EventClient) {
	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()
	close(client.C)
}

// Broadcast sends an event to all connected clients. Non-blocking: if a
// client's buffer is full the event is dropped for that client.
func (h *EventHub) Broadcast(ev AgenticEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		if c.WorkspaceSlug != "" && c.WorkspaceSlug != ev.Workspace {
			continue
		}
		select {
		case c.C <- ev:
		default:
			// Drop — client is too slow.
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *EventHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
