package graph

import (
	"context"
	"errors"
	"log/slog"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	coreg "github.com/neokapi/neokapi/core/graph"
)

// GraphSyncer syncs relational entity changes to graph nodes via event bus.
type GraphSyncer struct {
	store coreg.GraphStore
	bus   platev.EventBus
	sub   *platev.Subscription
}

// NewGraphSyncer creates a GraphSyncer that listens to content events
// and keeps the graph in sync.
//
// Durability: graph sync is state-advancing — a block event missed during a
// deploy rollover is silent graph drift with no reconcile pass to heal it — so
// it joins the "graph-syncer" consumer group (the group AD-012 documents):
// the resume position survives restarts and exactly one instance mirrors each
// event, instead of every replica racing the same writes. Replay-safe: node
// create/update/delete keyed by the stable block ID are idempotent, so a
// redelivered event mirrors the same node twice to the same effect.
func NewGraphSyncer(store coreg.GraphStore, bus platev.EventBus) *GraphSyncer {
	s := &GraphSyncer{store: store, bus: bus}
	s.sub = bus.SubscribeGroup("graph-syncer", s.handleEvent)
	return s
}

// handleEvent mirrors one block change into the graph, and reports whether it
// landed. Drift here has no reconcile pass to heal it, so a write that failed
// is worth redelivering — and can be, because the writes are keyed on the
// block id.
func (s *GraphSyncer) handleEvent(ev platev.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch ev.Type {
	case platev.EventBlockCreated:
		return s.onBlockCreated(ctx, ev)
	case platev.EventBlockUpdated:
		return s.onBlockUpdated(ctx, ev)
	case platev.EventBlockDeleted:
		return s.onBlockDeleted(ctx, ev)
	}
	return nil
}

func (s *GraphSyncer) onBlockCreated(ctx context.Context, ev platev.Event) error {
	blockID := ev.Data["block_id"]
	if blockID == "" {
		return nil
	}

	node := &coreg.Node{
		ID:    blockID,
		Label: "Concept",
		Properties: map[string]string{
			"project_id": ev.ProjectID,
			"name":       ev.Data["name"],
		},
	}

	// A node this event already created is not a failure to retry: the block
	// id is the node id, so the graph already says what this event says.
	if err := s.store.CreateNode(ctx, node); err != nil {
		slog.Info("graph sync: create node for block", "id", blockID, "error", err)
	}
	return nil
}

func (s *GraphSyncer) onBlockUpdated(ctx context.Context, ev platev.Event) error {
	blockID := ev.Data["block_id"]
	if blockID == "" {
		return nil
	}

	node := &coreg.Node{
		ID:    blockID,
		Label: "Concept",
		Properties: map[string]string{
			"project_id": ev.ProjectID,
			"name":       ev.Data["name"],
		},
	}

	err := s.store.UpdateNode(ctx, node)
	if errors.Is(err, coreg.ErrNodeNotFound) {
		// The create was missed — a block pushed before this syncer ran, or a
		// node wiped by a data reset while its block survived. The update
		// carries everything the node needs, so upsert rather than letting
		// the event fail (and redeliver) forever.
		err = s.store.CreateNode(ctx, node)
	}
	if err != nil {
		slog.Info("graph sync: update node for block", "id", blockID, "error", err)
		return err
	}
	return nil
}

func (s *GraphSyncer) onBlockDeleted(ctx context.Context, ev platev.Event) error {
	blockID := ev.Data["block_id"]
	if blockID == "" {
		return nil
	}

	if err := s.store.DeleteNode(ctx, blockID); err != nil {
		if errors.Is(err, coreg.ErrNodeNotFound) {
			return nil // already gone — this event's outcome holds
		}
		slog.Info("graph sync: delete node for block", "id", blockID, "error", err)
		return err
	}
	return nil
}

// Close unsubscribes from the event bus.
func (s *GraphSyncer) Close() {
	if s.sub != nil {
		s.bus.Unsubscribe(s.sub)
	}
}
