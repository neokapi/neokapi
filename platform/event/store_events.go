package event

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	platev "github.com/neokapi/neokapi/platform/event"
	"github.com/neokapi/neokapi/platform/store"
)

// EventEmittingStore wraps a ContentStore and emits events on mutations.
type EventEmittingStore struct {
	inner store.ContentStore
	bus   platev.EventBus
}

// NewEventEmittingStore wraps a ContentStore with event emission.
func NewEventEmittingStore(inner store.ContentStore, bus platev.EventBus) *EventEmittingStore {
	return &EventEmittingStore{inner: inner, bus: bus}
}

func (s *EventEmittingStore) CreateProject(ctx context.Context, p *store.Project) error {
	if err := s.inner.CreateProject(ctx, p); err != nil {
		return err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventProjectCreated,
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
	s.bus.Publish(platev.Event{
		Type:      platev.EventProjectUpdated,
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
	s.bus.Publish(platev.Event{
		Type:      platev.EventProjectDeleted,
		Source:    "store",
		ProjectID: id,
	})
	return nil
}

func (s *EventEmittingStore) ArchiveProject(ctx context.Context, id string) error {
	return s.inner.ArchiveProject(ctx, id)
}

func (s *EventEmittingStore) RestoreProject(ctx context.Context, id string) error {
	return s.inner.RestoreProject(ctx, id)
}

func (s *EventEmittingStore) ListArchivedProjects(ctx context.Context, workspaceID string) ([]*store.Project, error) {
	return s.inner.ListArchivedProjects(ctx, workspaceID)
}

// --- Stream management ---

func (s *EventEmittingStore) CreateStream(ctx context.Context, st *store.Stream) error {
	if err := s.inner.CreateStream(ctx, st); err != nil {
		return err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventStreamCreated,
		Source:    "store",
		ProjectID: st.ProjectID,
		Data:      map[string]string{"stream": st.Name, "parent": st.Parent},
	})
	return nil
}

func (s *EventEmittingStore) GetStream(ctx context.Context, projectID, name string) (*store.Stream, error) {
	return s.inner.GetStream(ctx, projectID, name)
}

func (s *EventEmittingStore) ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*store.Stream, error) {
	return s.inner.ListStreams(ctx, projectID, includeArchived)
}

func (s *EventEmittingStore) UpdateStream(ctx context.Context, st *store.Stream) error {
	return s.inner.UpdateStream(ctx, st)
}

func (s *EventEmittingStore) DeleteStream(ctx context.Context, projectID, name string) error {
	if err := s.inner.DeleteStream(ctx, projectID, name); err != nil {
		return err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventStreamDeleted,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"stream": name},
	})
	return nil
}

// --- Stream operations ---

func (s *EventEmittingStore) MergeStream(ctx context.Context, projectID, streamName string, opts store.MergeOptions) (*store.MergeResult, error) {
	result, err := s.inner.MergeStream(ctx, projectID, streamName, opts)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		s.bus.Publish(platev.Event{
			Type:      platev.EventStreamMerged,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"stream": streamName},
		})
	}
	return result, nil
}

func (s *EventEmittingStore) DiffStream(ctx context.Context, projectID, streamName string) (*store.StreamDiff, error) {
	return s.inner.DiffStream(ctx, projectID, streamName)
}

// --- Stream membership ---

func (s *EventEmittingStore) AddStreamMember(ctx context.Context, projectID, streamName, userID string) error {
	return s.inner.AddStreamMember(ctx, projectID, streamName, userID)
}

func (s *EventEmittingStore) RemoveStreamMember(ctx context.Context, projectID, streamName, userID string) error {
	return s.inner.RemoveStreamMember(ctx, projectID, streamName, userID)
}

func (s *EventEmittingStore) ListStreamMembers(ctx context.Context, projectID, streamName string) ([]string, error) {
	return s.inner.ListStreamMembers(ctx, projectID, streamName)
}

// --- Collections (project-scoped) ---

func (s *EventEmittingStore) CreateCollection(ctx context.Context, c *store.Collection) error {
	if err := s.inner.CreateCollection(ctx, c); err != nil {
		return err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventCollectionCreated,
		Source:    "store",
		ProjectID: c.ProjectID,
		Data:      map[string]string{"collection_id": c.ID, "name": c.Name, "kind": string(c.Kind)},
	})
	return nil
}

func (s *EventEmittingStore) GetCollection(ctx context.Context, projectID, collectionID string) (*store.Collection, error) {
	return s.inner.GetCollection(ctx, projectID, collectionID)
}

func (s *EventEmittingStore) GetCollectionByName(ctx context.Context, projectID, name, stream string) (*store.Collection, error) {
	return s.inner.GetCollectionByName(ctx, projectID, name, stream)
}

func (s *EventEmittingStore) GetDefaultCollection(ctx context.Context, projectID string) (*store.Collection, error) {
	return s.inner.GetDefaultCollection(ctx, projectID)
}

func (s *EventEmittingStore) ListCollections(ctx context.Context, projectID, stream string) ([]*store.Collection, error) {
	return s.inner.ListCollections(ctx, projectID, stream)
}

func (s *EventEmittingStore) UpdateCollection(ctx context.Context, c *store.Collection) error {
	if err := s.inner.UpdateCollection(ctx, c); err != nil {
		return err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventCollectionUpdated,
		Source:    "store",
		ProjectID: c.ProjectID,
		Data:      map[string]string{"collection_id": c.ID, "name": c.Name},
	})
	return nil
}

func (s *EventEmittingStore) DeleteCollection(ctx context.Context, projectID, collectionID string) error {
	if err := s.inner.DeleteCollection(ctx, projectID, collectionID); err != nil {
		return err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventCollectionDeleted,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"collection_id": collectionID},
	})
	return nil
}

// --- Items (stream-scoped) ---

func (s *EventEmittingStore) StoreItem(ctx context.Context, projectID, stream string, item *store.Item) error {
	if err := s.inner.StoreItem(ctx, projectID, stream, item); err != nil {
		return err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventItemCreated,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"item_name": item.Name, "stream": stream, "format": item.Format},
	})
	return nil
}

func (s *EventEmittingStore) GetItem(ctx context.Context, projectID, stream, itemName string) (*store.Item, error) {
	return s.inner.GetItem(ctx, projectID, stream, itemName)
}

func (s *EventEmittingStore) ListItems(ctx context.Context, projectID, stream string) ([]*store.Item, error) {
	return s.inner.ListItems(ctx, projectID, stream)
}

func (s *EventEmittingStore) DeleteItem(ctx context.Context, projectID, stream, itemName string) error {
	if err := s.inner.DeleteItem(ctx, projectID, stream, itemName); err != nil {
		return err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventItemDeleted,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"item_name": itemName, "stream": stream},
	})
	return nil
}

func (s *EventEmittingStore) GetItemByID(ctx context.Context, projectID, stream, itemID string) (*store.Item, error) {
	return s.inner.GetItemByID(ctx, projectID, stream, itemID)
}

// --- Blocks (stream-scoped) ---

func (s *EventEmittingStore) StoreBlocks(ctx context.Context, projectID, stream string, blocks []*model.Block) error {
	if err := s.inner.StoreBlocks(ctx, projectID, stream, blocks); err != nil {
		return err
	}
	for _, b := range blocks {
		s.bus.Publish(platev.Event{
			Type:      platev.EventBlockUpdated,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"block_id": b.ID},
		})
	}
	return nil
}

func (s *EventEmittingStore) StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error {
	if err := s.inner.StoreBlocksForItem(ctx, projectID, stream, itemName, blocks); err != nil {
		return err
	}
	for _, b := range blocks {
		s.bus.Publish(platev.Event{
			Type:      platev.EventBlockUpdated,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"block_id": b.ID, "item_name": itemName},
		})
	}
	return nil
}

func (s *EventEmittingStore) GetBlock(ctx context.Context, projectID, stream, blockID string) (*store.StoredBlock, error) {
	return s.inner.GetBlock(ctx, projectID, stream, blockID)
}

func (s *EventEmittingStore) GetBlocks(ctx context.Context, query store.BlockQuery) ([]*store.StoredBlock, error) {
	return s.inner.GetBlocks(ctx, query)
}

func (s *EventEmittingStore) GetBlockStats(ctx context.Context, projectID, stream string) ([]store.BlockStatRow, error) {
	return s.inner.GetBlockStats(ctx, projectID, stream)
}

func (s *EventEmittingStore) DeleteBlock(ctx context.Context, projectID, stream, blockID string) error {
	if err := s.inner.DeleteBlock(ctx, projectID, stream, blockID); err != nil {
		return err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventBlockDeleted,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"block_id": blockID},
	})
	return nil
}

// --- Versions (stream-scoped) ---

func (s *EventEmittingStore) CreateVersion(ctx context.Context, projectID, stream, label, description string) (*store.Version, error) {
	v, err := s.inner.CreateVersion(ctx, projectID, stream, label, description)
	if err != nil {
		return nil, err
	}
	s.bus.Publish(platev.Event{
		Type:      platev.EventVersionCreated,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"version_id": v.ID, "label": label},
	})
	return v, nil
}

func (s *EventEmittingStore) GetVersion(ctx context.Context, versionID string) (*store.Version, error) {
	return s.inner.GetVersion(ctx, versionID)
}

func (s *EventEmittingStore) ListVersions(ctx context.Context, projectID, stream string) ([]*store.Version, error) {
	return s.inner.ListVersions(ctx, projectID, stream)
}

func (s *EventEmittingStore) Diff(ctx context.Context, from, to string) (*store.VersionDiff, error) {
	return s.inner.Diff(ctx, from, to)
}

// --- Block notes (stream-scoped) ---

func (s *EventEmittingStore) AddBlockNote(ctx context.Context, projectID, stream, blockID string, note model.BlockNote) error {
	return s.inner.AddBlockNote(ctx, projectID, stream, blockID, note)
}

func (s *EventEmittingStore) ListBlockNotes(ctx context.Context, projectID, stream, blockID string) ([]model.BlockNote, error) {
	return s.inner.ListBlockNotes(ctx, projectID, stream, blockID)
}

func (s *EventEmittingStore) DeleteBlockNote(ctx context.Context, projectID, stream, noteID string) error {
	return s.inner.DeleteBlockNote(ctx, projectID, stream, noteID)
}

// --- Block history (stream-scoped) ---

func (s *EventEmittingStore) GetBlockHistory(ctx context.Context, projectID, stream, blockID string, locale string, limit int) ([]store.BlockHistoryEntry, error) {
	return s.inner.GetBlockHistory(ctx, projectID, stream, blockID, locale, limit)
}

// --- Change log (stream-scoped) ---

func (s *EventEmittingStore) GetChanges(ctx context.Context, projectID, stream string, sinceCursor int64, locales []string, limit int) (*store.ChangeSet, error) {
	return s.inner.GetChanges(ctx, projectID, stream, sinceCursor, locales, limit)
}

func (s *EventEmittingStore) LatestCursor(ctx context.Context, projectID, stream string) (int64, error) {
	return s.inner.LatestCursor(ctx, projectID, stream)
}

func (s *EventEmittingStore) CompactChangeLog(ctx context.Context, projectID, stream string, retainDays int) (int64, error) {
	return s.inner.CompactChangeLog(ctx, projectID, stream, retainDays)
}

func (s *EventEmittingStore) Close() error {
	return s.inner.Close()
}

// Ensure EventEmittingStore implements ContentStore.
var _ store.ContentStore = (*EventEmittingStore)(nil)
