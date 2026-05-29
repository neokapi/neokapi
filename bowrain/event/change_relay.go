package event

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
)

// ChangeRelay is a unified, in-process fan-out of platform events to attached
// clients (SSE for web, gRPC WatchProject for desktop). It subscribes to the
// EventBus once per server instance and forwards every relevant event to the
// clients registered on that instance, filtered by workspace and (optionally)
// project.
//
// It is NATS-aware by construction: it attaches via EventBus.SubscribeAll,
// which on the distributed NATS bus creates a unique (non-competing) consumer.
// Every server instance therefore receives a copy of every event and relays it
// to its own locally-connected clients — exactly the semantics WatchProject
// already relies on. (Using SubscribeGroup would hand each event to only one
// instance, starving clients connected elsewhere.)
//
// Delivery to clients is asynchronous and strictly non-blocking: a slow or
// stuck client never blocks the event bus or other clients. When a client's
// buffer is full the relayed event is dropped for that client (the client is
// expected to refetch authoritative state on the next event or reconnect).
type ChangeRelay struct {
	bus      platev.EventBus
	sub      *platev.Subscription
	resolver ProjectWorkspaceResolver

	mu      sync.RWMutex
	clients map[string]*relayClient

	// wsCache memoizes project-ID → workspace-ID lookups so workspace-scoped
	// clients can be matched without a store round-trip per event.
	wsCache sync.Map // map[string]string
}

// ProjectWorkspaceResolver resolves a project ID to its owning workspace ID.
// Implemented by the ContentStore-backed adapter in the server package.
type ProjectWorkspaceResolver interface {
	WorkspaceForProject(ctx context.Context, projectID string) (string, error)
}

type relayClient struct {
	id          string
	workspaceID string // required: clients are always workspace-scoped
	projectID   string // optional: empty = all projects in the workspace
	ch          chan ChangeEvent
}

// ChangeEvent is the wire shape relayed to clients. It is a flattened,
// client-friendly projection of a platform event: the discriminating type plus
// the identifying fields a view needs to decide what to refetch.
type ChangeEvent struct {
	Type       string `json:"type"`
	ProjectID  string `json:"projectId,omitempty"`
	Stream     string `json:"stream,omitempty"`
	ItemName   string `json:"itemName,omitempty"`
	BlockID    string `json:"blockId,omitempty"`
	ChangedBy  string `json:"changedBy,omitempty"`
	ChangeType string `json:"changeType,omitempty"`
	Actor      string `json:"actor,omitempty"`
}

// NewChangeRelay creates a relay and attaches it to the bus. The resolver may
// be nil; when nil, workspace-scoped clients (no project filter) receive only
// events that already carry a workspace_id/workspace_slug in their Data, and
// project-scoped clients still work via direct project-ID matching.
func NewChangeRelay(bus platev.EventBus, resolver ProjectWorkspaceResolver) *ChangeRelay {
	r := &ChangeRelay{
		bus:      bus,
		resolver: resolver,
		clients:  make(map[string]*relayClient),
	}
	if bus != nil {
		r.sub = bus.SubscribeAll(r.dispatch)
	}
	return r
}

// Subscribe registers a client for events in the given workspace and (optional)
// project. The returned channel delivers ChangeEvents until Unsubscribe is
// called. The buffer is sized to absorb short bursts; on overflow events are
// dropped for this client only.
func (r *ChangeRelay) Subscribe(workspaceID, projectID string) (string, <-chan ChangeEvent) {
	c := &relayClient{
		id:          id.New(),
		workspaceID: workspaceID,
		projectID:   projectID,
		ch:          make(chan ChangeEvent, 64),
	}
	r.mu.Lock()
	r.clients[c.id] = c
	r.mu.Unlock()
	return c.id, c.ch
}

// Unsubscribe removes a client and closes its channel.
func (r *ChangeRelay) Unsubscribe(clientID string) {
	r.mu.Lock()
	c, ok := r.clients[clientID]
	if ok {
		delete(r.clients, clientID)
	}
	r.mu.Unlock()
	if ok {
		close(c.ch)
	}
}

// ClientCount returns the number of attached clients (for tests/metrics).
func (r *ChangeRelay) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

// Close detaches from the bus. Attached client channels are closed.
func (r *ChangeRelay) Close() {
	if r.bus != nil && r.sub != nil {
		r.bus.Unsubscribe(r.sub)
	}
	r.mu.Lock()
	for _, c := range r.clients {
		close(c.ch)
	}
	r.clients = make(map[string]*relayClient)
	r.mu.Unlock()
}

// dispatch is the EventBus handler. It runs on the bus's subscriber goroutine,
// so it must never block: it only does cheap matching + a non-blocking send.
func (r *ChangeRelay) dispatch(ev platev.Event) {
	ce, ok := relayableEvent(ev)
	if !ok {
		return
	}

	// Resolve the event's workspace once (cheap, cached) so workspace-scoped
	// clients can be matched. Project-scoped clients match on project ID and
	// do not need this.
	eventWS := r.eventWorkspaceID(ev)

	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.clients {
		if !clientMatches(c, ev.ProjectID, eventWS) {
			continue
		}
		select {
		case c.ch <- ce:
		default:
			slog.Warn("change relay dropping event: client buffer full",
				"event_type", ev.Type, "client_id", c.id, "project_id", ev.ProjectID)
		}
	}
}

// clientMatches reports whether the event (with project ID and resolved
// workspace ID) should be delivered to the client.
func clientMatches(c *relayClient, eventProjectID, eventWorkspaceID string) bool {
	if c.projectID != "" {
		// Project-scoped: only this project's events.
		return eventProjectID == c.projectID
	}
	// Workspace-scoped: events for any project in the workspace. When the
	// event's workspace can't be resolved (no project, no cached mapping),
	// fall back to project-less events (workspace-global), delivered to all
	// workspace clients so nothing silently goes stale.
	if eventWorkspaceID == "" {
		return eventProjectID == ""
	}
	return eventWorkspaceID == c.workspaceID
}

// eventWorkspaceID resolves the workspace owning the event, preferring the
// event payload, then the resolver cache, then a one-shot resolver lookup.
func (r *ChangeRelay) eventWorkspaceID(ev platev.Event) string {
	if ev.Data != nil {
		if ws := ev.Data["workspace_id"]; ws != "" {
			return ws
		}
	}
	if ev.ProjectID == "" {
		return ""
	}
	if v, ok := r.wsCache.Load(ev.ProjectID); ok {
		return v.(string)
	}
	if r.resolver == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ws, err := r.resolver.WorkspaceForProject(ctx, ev.ProjectID)
	if err != nil || ws == "" {
		return ""
	}
	r.wsCache.Store(ev.ProjectID, ws)
	return ws
}

// relayableEvent maps a platform event to the relay wire shape. It returns
// ok=false for events that are not interesting to view freshness (internal
// agent chatter, presence — presence already flows over its own channels).
func relayableEvent(ev platev.Event) (ChangeEvent, bool) {
	t := string(ev.Type)

	// Presence flows over the gRPC presence variant + Yjs awareness; the relay
	// is for content/state freshness, not cursors.
	if strings.HasPrefix(t, "editor.presence.") {
		return ChangeEvent{}, false
	}
	// Agent conversation chatter is delivered over the @bravo SSE stream.
	if strings.HasPrefix(t, "agent.") {
		return ChangeEvent{}, false
	}

	ce := ChangeEvent{
		Type:       t,
		ProjectID:  ev.ProjectID,
		Actor:      ev.Actor,
		ChangedBy:  ev.Data["changed_by"],
		ChangeType: ev.Data["change_type"],
	}
	if ev.Data != nil {
		ce.Stream = ev.Data["stream"]
		ce.ItemName = ev.Data["item_name"]
		ce.BlockID = ev.Data["block_id"]
	}
	return ce, true
}
