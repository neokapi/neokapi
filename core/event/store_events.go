package event

import (
	"context"
	"io"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/store"
)

// EventEmittingStore wraps a ContentStore and emits events on mutations.
type EventEmittingStore struct {
	inner store.ContentStore
	bus   EventBus
}

// NewEventEmittingStore wraps a ContentStore with event emission.
func NewEventEmittingStore(inner store.ContentStore, bus EventBus) *EventEmittingStore {
	return &EventEmittingStore{inner: inner, bus: bus}
}

func (s *EventEmittingStore) CreateProject(ctx context.Context, p *store.Project) error {
	if err := s.inner.CreateProject(ctx, p); err != nil {
		return err
	}
	s.bus.Publish(Event{
		Type:      EventProjectCreated,
		Source:    "store",
		ProjectID: p.ID,
		Data:      map[string]string{"name": p.Name},
	})
	return nil
}

func (s *EventEmittingStore) GetProject(ctx context.Context, id string) (*store.Project, error) {
	return s.inner.GetProject(ctx, id)
}

func (s *EventEmittingStore) ListProjects(ctx context.Context) ([]*store.Project, error) {
	return s.inner.ListProjects(ctx)
}

func (s *EventEmittingStore) UpdateProject(ctx context.Context, p *store.Project) error {
	if err := s.inner.UpdateProject(ctx, p); err != nil {
		return err
	}
	s.bus.Publish(Event{
		Type:      EventProjectUpdated,
		Source:    "store",
		ProjectID: p.ID,
		Data:      map[string]string{"name": p.Name},
	})
	return nil
}

func (s *EventEmittingStore) DeleteProject(ctx context.Context, id string) error {
	if err := s.inner.DeleteProject(ctx, id); err != nil {
		return err
	}
	s.bus.Publish(Event{
		Type:      EventProjectDeleted,
		Source:    "store",
		ProjectID: id,
	})
	return nil
}

func (s *EventEmittingStore) StoreBlocks(ctx context.Context, projectID string, blocks []*model.Block) error {
	if err := s.inner.StoreBlocks(ctx, projectID, blocks); err != nil {
		return err
	}
	for _, b := range blocks {
		s.bus.Publish(Event{
			Type:      EventBlockUpdated,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"block_id": b.ID},
		})
	}
	return nil
}

func (s *EventEmittingStore) GetBlock(ctx context.Context, projectID, blockID string) (*store.StoredBlock, error) {
	return s.inner.GetBlock(ctx, projectID, blockID)
}

func (s *EventEmittingStore) GetBlocks(ctx context.Context, query store.BlockQuery) ([]*store.StoredBlock, error) {
	return s.inner.GetBlocks(ctx, query)
}

func (s *EventEmittingStore) DeleteBlock(ctx context.Context, projectID, blockID string) error {
	if err := s.inner.DeleteBlock(ctx, projectID, blockID); err != nil {
		return err
	}
	s.bus.Publish(Event{
		Type:      EventBlockDeleted,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"block_id": blockID},
	})
	return nil
}

func (s *EventEmittingStore) CreateVersion(ctx context.Context, projectID, label, description string) (*store.Version, error) {
	v, err := s.inner.CreateVersion(ctx, projectID, label, description)
	if err != nil {
		return nil, err
	}
	s.bus.Publish(Event{
		Type:      EventVersionCreated,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"version_id": v.ID, "label": label},
	})
	return v, nil
}

func (s *EventEmittingStore) GetVersion(ctx context.Context, versionID string) (*store.Version, error) {
	return s.inner.GetVersion(ctx, versionID)
}

func (s *EventEmittingStore) ListVersions(ctx context.Context, projectID string) ([]*store.Version, error) {
	return s.inner.ListVersions(ctx, projectID)
}

func (s *EventEmittingStore) Diff(ctx context.Context, from, to string) (*store.VersionDiff, error) {
	return s.inner.Diff(ctx, from, to)
}

func (s *EventEmittingStore) ExportKAZ(ctx context.Context, projectID string, w io.Writer) error {
	return s.inner.ExportKAZ(ctx, projectID, w)
}

func (s *EventEmittingStore) ImportKAZ(ctx context.Context, r io.Reader) (string, error) {
	return s.inner.ImportKAZ(ctx, r)
}

func (s *EventEmittingStore) Close() error {
	return s.inner.Close()
}

// Ensure EventEmittingStore implements ContentStore.
var _ store.ContentStore = (*EventEmittingStore)(nil)
