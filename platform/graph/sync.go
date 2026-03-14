package graph

import (
	"context"
	"log"
	"time"

	coreg "github.com/neokapi/neokapi/core/graph"
	platev "github.com/neokapi/neokapi/platform/event"
)

// GraphSyncer syncs relational entity changes to graph nodes via event bus.
type GraphSyncer struct {
	store coreg.GraphStore
	bus   platev.EventBus
	sub   *platev.Subscription
}

// NewGraphSyncer creates a GraphSyncer that listens to content events
// and keeps the graph in sync.
func NewGraphSyncer(store coreg.GraphStore, bus platev.EventBus) *GraphSyncer {
	s := &GraphSyncer{store: store, bus: bus}
	s.sub = bus.SubscribeAll(s.handleEvent)
	return s
}

func (s *GraphSyncer) handleEvent(ev platev.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch ev.Type {
	case platev.EventBlockCreated:
		s.onBlockCreated(ctx, ev)
	case platev.EventBlockUpdated:
		s.onBlockUpdated(ctx, ev)
	case platev.EventBlockDeleted:
		s.onBlockDeleted(ctx, ev)
	}
}

func (s *GraphSyncer) onBlockCreated(ctx context.Context, ev platev.Event) {
	blockID := ev.Data["block_id"]
	if blockID == "" {
		return
	}

	node := &coreg.Node{
		ID:    blockID,
		Label: "Concept",
		Properties: map[string]string{
			"project_id": ev.ProjectID,
			"name":       ev.Data["name"],
		},
	}

	if err := s.store.CreateNode(ctx, node); err != nil {
		log.Printf("graph sync: create node for block %s: %v", blockID, err)
	}
}

func (s *GraphSyncer) onBlockUpdated(ctx context.Context, ev platev.Event) {
	blockID := ev.Data["block_id"]
	if blockID == "" {
		return
	}

	node := &coreg.Node{
		ID:    blockID,
		Label: "Concept",
		Properties: map[string]string{
			"project_id": ev.ProjectID,
			"name":       ev.Data["name"],
		},
	}

	if err := s.store.UpdateNode(ctx, node); err != nil {
		log.Printf("graph sync: update node for block %s: %v", blockID, err)
	}
}

func (s *GraphSyncer) onBlockDeleted(ctx context.Context, ev platev.Event) {
	blockID := ev.Data["block_id"]
	if blockID == "" {
		return
	}

	if err := s.store.DeleteNode(ctx, blockID); err != nil {
		log.Printf("graph sync: delete node for block %s: %v", blockID, err)
	}
}

// Close unsubscribes from the event bus.
func (s *GraphSyncer) Close() {
	if s.sub != nil {
		s.bus.Unsubscribe(s.sub)
	}
}
