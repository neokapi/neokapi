package event

import (
	platev "github.com/neokapi/neokapi/platform/event"
)

// GraphSyncHandler handles events that require graph synchronization.
// When concepts or terms are created/updated/deleted, the graph store
// should be updated to maintain the derived graph view.
type GraphSyncHandler struct {
	bus platev.EventBus
	sub *platev.Subscription
}

// NewGraphSyncHandler creates a GraphSyncHandler subscribed to all events.
func NewGraphSyncHandler(bus platev.EventBus) *GraphSyncHandler {
	h := &GraphSyncHandler{bus: bus}
	h.sub = bus.SubscribeAll(h.handleEvent)
	return h
}

func (h *GraphSyncHandler) handleEvent(ev platev.Event) {
	// TODO: Implement graph sync for concept/term CRUD events
	// This will be wired up when the graph store is available
}

// Close unsubscribes from the event bus.
func (h *GraphSyncHandler) Close() {
	if h.sub != nil {
		h.bus.Unsubscribe(h.sub)
	}
}
