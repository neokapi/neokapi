package event

import (
	"context"
	"fmt"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/venue"
)

var _ store.BlockWriteStore = (*EventEmittingStore)(nil)

// ItemBlocks forwards the optional BlockWriteStore capability.
func (s *EventEmittingStore) ItemBlocks(ctx context.Context, projectID, stream, itemName string, keys []string) ([]*venue.StoredBlock, error) {
	ws, ok := s.inner.(store.BlockWriteStore)
	if !ok {
		return nil, fmt.Errorf("content store %T keeps no held block writes", s.inner)
	}
	return ws.ItemBlocks(ctx, projectID, stream, itemName, keys)
}

// BeginBlockWrite forwards the optional BlockWriteStore capability. The write
// it returns announces each block it stored once it commits, as UpdateBlock
// does.
func (s *EventEmittingStore) BeginBlockWrite(ctx context.Context, projectID, stream string) (store.BlockWrite, error) {
	ws, ok := s.inner.(store.BlockWriteStore)
	if !ok {
		return nil, fmt.Errorf("content store %T keeps no held block writes", s.inner)
	}
	w, err := ws.BeginBlockWrite(ctx, projectID, stream)
	if err != nil {
		return nil, err
	}
	return &eventBlockWrite{inner: w, store: s, ctx: ctx, projectID: projectID}, nil
}

// eventBlockWrite publishes block.updated for each block a committed write
// stored.
type eventBlockWrite struct {
	inner     store.BlockWrite
	store     *EventEmittingStore
	ctx       context.Context //nolint:containedctx // the commit's events carry the actor of the write that began it
	projectID string
	stored    []string
}

func (w *eventBlockWrite) Hold(ctx context.Context, itemName string, keys []string) ([]*venue.StoredBlock, error) {
	return w.inner.Hold(ctx, itemName, keys)
}

func (w *eventBlockWrite) Store(ctx context.Context, blocks []*venue.StoredBlock) error {
	if err := w.inner.Store(ctx, blocks); err != nil {
		return err
	}
	for _, sb := range blocks {
		if sb != nil && sb.Block != nil {
			w.stored = append(w.stored, sb.Block.ID)
		}
	}
	return nil
}

func (w *eventBlockWrite) Commit() error {
	if err := w.inner.Commit(); err != nil {
		return err
	}
	for _, id := range w.stored {
		w.store.publish(w.ctx, platev.Event{
			Type:      platev.EventBlockUpdated,
			Source:    "store",
			ProjectID: w.projectID,
			Data:      map[string]string{"block_id": id},
		})
	}
	w.stored = nil
	return nil
}

func (w *eventBlockWrite) Rollback() error { return w.inner.Rollback() }
