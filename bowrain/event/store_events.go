package event

import (
	"context"
	"fmt"
	"strconv"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
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

// Unwrap names the store this one decorates, so a caller probing for an
// optional capability finds it where it is implemented rather than concluding
// the decorator has none (store.ShipVerdicts). Event emission is a write-path
// concern: a capability reached this way still writes through the same
// database, it simply does not announce itself on the bus.
func (s *EventEmittingStore) Unwrap() store.ContentStore { return s.inner }

func (s *EventEmittingStore) publish(ctx context.Context, ev platev.Event) {
	if ev.Actor == "" {
		ev.Actor = platev.ActorFromContext(ctx)
	}
	if ev.Data != nil {
		if _, ok := ev.Data["actor_name"]; !ok {
			if name := platev.ActorNameFromContext(ctx); name != "" {
				ev.Data["actor_name"] = name
			}
		}
	}
	if meta := platev.RequestMetaFromContext(ctx); meta != (platev.RequestMeta{}) {
		if ev.RequestID == "" {
			ev.RequestID = meta.RequestID
		}
		if ev.IP == "" {
			ev.IP = meta.IP
		}
		if ev.UserAgent == "" {
			ev.UserAgent = meta.UserAgent
		}
	}
	s.bus.Publish(ev)
}

func (s *EventEmittingStore) CreateProject(ctx context.Context, p *store.Project) error {
	if err := s.inner.CreateProject(ctx, p); err != nil {
		return err
	}
	s.publish(ctx, platev.Event{
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
	s.publish(ctx, platev.Event{
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
	s.publish(ctx, platev.Event{
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

// CreateStream forwards the optional StreamBranchStore capability. The
// decorator satisfies the interface whether or not the store it wraps does, so
// a caller asserting for it on the decorator gets the same answer it would get
// from the store underneath — which is the point of asserting for a capability
// rather than for a concrete type.
func (s *EventEmittingStore) CreateStream(ctx context.Context, st *store.Stream) error {
	bs, ok := s.inner.(store.StreamBranchStore)
	if !ok {
		return fmt.Errorf("content store %T does not branch", s.inner)
	}
	if err := bs.CreateStream(ctx, st); err != nil {
		return err
	}
	s.publish(ctx, platev.Event{
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
	s.publish(ctx, platev.Event{
		Type:      platev.EventStreamDeleted,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"stream": name},
	})
	return nil
}

// --- Stream operations ---

func (s *EventEmittingStore) MergeStream(ctx context.Context, projectID, streamName string, opts store.MergeOptions) (*store.MergeResult, error) {
	bs, ok := s.inner.(store.StreamBranchStore)
	if !ok {
		return nil, fmt.Errorf("content store %T does not branch", s.inner)
	}
	result, err := bs.MergeStream(ctx, projectID, streamName, opts)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		// Get the stream to find parent name for the tag.
		stream, _ := s.inner.GetStream(ctx, projectID, streamName)
		parentName := "main"
		if stream != nil && stream.Parent != "" {
			parentName = stream.Parent
		}

		s.publish(ctx, platev.Event{
			Type:      platev.EventStreamMerged,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"stream": streamName, "parent": parentName},
		})

		// Auto-create a merge tag.
		cursor, _ := s.inner.LatestCursor(ctx, projectID, streamName)
		actor := platev.ActorFromContext(ctx)
		tagName := fmt.Sprintf("merged-%s-%s", parentName, time.Now().UTC().Format("20060102-150405"))
		_ = s.inner.CreateStreamTag(ctx, &store.StreamTag{
			ProjectID: projectID,
			Stream:    streamName,
			Name:      tagName,
			Kind:      store.TagKindMerge,
			Cursor:    cursor,
			Metadata: map[string]string{
				"target_stream":   parentName,
				"merged_blocks":   strconv.Itoa(result.MergedBlocks),
				"added_blocks":    strconv.Itoa(result.AddedBlocks),
				"modified_blocks": strconv.Itoa(result.ModifiedBlocks),
				"removed_blocks":  strconv.Itoa(result.RemovedBlocks),
			},
			CreatedBy: actor,
		})
	}
	return result, nil
}

func (s *EventEmittingStore) DiffStream(ctx context.Context, projectID, streamName string) (*store.StreamDiff, error) {
	bs, ok := s.inner.(store.StreamBranchStore)
	if !ok {
		return nil, fmt.Errorf("content store %T does not branch", s.inner)
	}
	return bs.DiffStream(ctx, projectID, streamName)
}

// --- Stream lock ---

func (s *EventEmittingStore) LockStream(ctx context.Context, projectID, streamName, userID string) error {
	if err := s.inner.LockStream(ctx, projectID, streamName, userID); err != nil {
		return err
	}
	s.publish(ctx, platev.Event{
		Type:      platev.EventStreamLocked,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"stream": streamName, "locked_by": userID},
	})
	return nil
}

func (s *EventEmittingStore) UnlockStream(ctx context.Context, projectID, streamName string) error {
	if err := s.inner.UnlockStream(ctx, projectID, streamName); err != nil {
		return err
	}
	s.publish(ctx, platev.Event{
		Type:      platev.EventStreamUnlocked,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"stream": streamName},
	})
	return nil
}

// --- Stream tags ---

func (s *EventEmittingStore) CreateStreamTag(ctx context.Context, tag *store.StreamTag) error {
	if err := s.inner.CreateStreamTag(ctx, tag); err != nil {
		return err
	}
	s.publish(ctx, platev.Event{
		Type:      platev.EventStreamTagged,
		Source:    "store",
		ProjectID: tag.ProjectID,
		Data:      map[string]string{"stream": tag.Stream, "tag": tag.Name, "kind": string(tag.Kind)},
	})
	return nil
}

func (s *EventEmittingStore) ListStreamTags(ctx context.Context, projectID, stream string) ([]*store.StreamTag, error) {
	return s.inner.ListStreamTags(ctx, projectID, stream)
}

func (s *EventEmittingStore) GetStreamTag(ctx context.Context, projectID, stream, tagName string) (*store.StreamTag, error) {
	return s.inner.GetStreamTag(ctx, projectID, stream, tagName)
}

func (s *EventEmittingStore) DeleteStreamTag(ctx context.Context, projectID, stream, tagName string) error {
	return s.inner.DeleteStreamTag(ctx, projectID, stream, tagName)
}

func (s *EventEmittingStore) ListProjectTags(ctx context.Context, projectID string, kind store.StreamTagKind) ([]*store.StreamTag, error) {
	return s.inner.ListProjectTags(ctx, projectID, kind)
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
	s.publish(ctx, platev.Event{
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
	s.publish(ctx, platev.Event{
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
	s.publish(ctx, platev.Event{
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
	s.publish(ctx, platev.Event{
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
	s.publish(ctx, platev.Event{
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
		s.publish(ctx, platev.Event{
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
		s.publish(ctx, platev.Event{
			Type:      platev.EventBlockUpdated,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"block_id": b.ID, "item_name": itemName},
		})
	}
	return nil
}

// PruneItemBlocks emits one event for the item rather than one per removed
// block: a prune's subject is the item, and the ids it drops name rows a
// subscriber can no longer read. What a surface needs is to refetch the item,
// which is what this tells it to do.
func (s *EventEmittingStore) PruneItemBlocks(ctx context.Context, projectID, stream, itemName string, keep []string) (int, error) {
	n, err := s.inner.PruneItemBlocks(ctx, projectID, stream, itemName, keep)
	if err != nil || n == 0 {
		return n, err
	}
	s.publish(ctx, platev.Event{
		Type:      platev.EventBlockUpdated,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"item_name": itemName},
	})
	return n, nil
}

func (s *EventEmittingStore) GetBlock(ctx context.Context, projectID, stream, blockID string) (*venue.StoredBlock, error) {
	return s.inner.GetBlock(ctx, projectID, stream, blockID)
}

func (s *EventEmittingStore) GetBlocks(ctx context.Context, query store.BlockQuery) ([]*venue.StoredBlock, error) {
	return s.inner.GetBlocks(ctx, query)
}

func (s *EventEmittingStore) CountBlocks(ctx context.Context, query store.BlockQuery) (store.BlockCounts, error) {
	return s.inner.CountBlocks(ctx, query)
}

func (s *EventEmittingStore) GetBlockStats(ctx context.Context, projectID, stream string) ([]store.BlockStatRow, error) {
	return s.inner.GetBlockStats(ctx, projectID, stream)
}

func (s *EventEmittingStore) ListPendingReview(ctx context.Context, query store.PendingReviewQuery) ([]store.PendingReviewRef, int, error) {
	return s.inner.ListPendingReview(ctx, query)
}

func (s *EventEmittingStore) DeleteBlock(ctx context.Context, projectID, stream, blockID string) error {
	if err := s.inner.DeleteBlock(ctx, projectID, stream, blockID); err != nil {
		return err
	}
	s.publish(ctx, platev.Event{
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
	s.publish(ctx, platev.Event{
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

// ---------------------------------------------------------------------------
// Asset CRUD (Bowrain AD-007) — delegate to inner store
// ---------------------------------------------------------------------------

func (s *EventEmittingStore) StoreAsset(ctx context.Context, projectID, stream string, asset *venue.Asset) error {
	return s.inner.StoreAsset(ctx, projectID, stream, asset)
}

func (s *EventEmittingStore) GetAsset(ctx context.Context, projectID, stream, assetID string) (*venue.Asset, error) {
	return s.inner.GetAsset(ctx, projectID, stream, assetID)
}

func (s *EventEmittingStore) ListAssets(ctx context.Context, projectID, stream, itemName string) ([]*venue.Asset, error) {
	return s.inner.ListAssets(ctx, projectID, stream, itemName)
}

func (s *EventEmittingStore) DeleteAsset(ctx context.Context, projectID, stream, assetID string) error {
	return s.inner.DeleteAsset(ctx, projectID, stream, assetID)
}

func (s *EventEmittingStore) StoreAssetVariant(ctx context.Context, projectID string, variant *venue.AssetVariant) error {
	return s.inner.StoreAssetVariant(ctx, projectID, variant)
}

func (s *EventEmittingStore) GetAssetVariant(ctx context.Context, projectID, assetID, locale string) (*venue.AssetVariant, error) {
	return s.inner.GetAssetVariant(ctx, projectID, assetID, locale)
}

func (s *EventEmittingStore) ListAssetVariants(ctx context.Context, projectID, assetID string) ([]*venue.AssetVariant, error) {
	return s.inner.ListAssetVariants(ctx, projectID, assetID)
}

// Ensure EventEmittingStore implements ContentStore.
var _ store.ContentStore = (*EventEmittingStore)(nil)

// UpsertUnitDecisions forwards the optional DecisionStore capability to the
// wrapped store. The wrapper must carry it explicitly: a `store.(DecisionStore)`
// assertion sees the wrapper's method set, not the inner store's, so without
// this the capability silently vanished exactly where the server needed it —
// the editor's review handler — while the worker, holding the raw store, kept
// it. No event is emitted here; decision events live in block_history.
func (s *EventEmittingStore) UpsertUnitDecisions(ctx context.Context, projectID, stream string, decisions []venue.UnitDecision) (int, error) {
	ds, ok := s.inner.(store.DecisionStore)
	if !ok {
		return 0, fmt.Errorf("content store %T keeps no decision ledger", s.inner)
	}
	return ds.UpsertUnitDecisions(ctx, projectID, stream, decisions)
}

// ListUnitDecisions forwards the optional DecisionStore capability.
func (s *EventEmittingStore) ListUnitDecisions(ctx context.Context, projectID, stream string) ([]venue.UnitDecision, error) {
	ds, ok := s.inner.(store.DecisionStore)
	if !ok {
		return nil, nil
	}
	return ds.ListUnitDecisions(ctx, projectID, stream)
}

// GetUnitDecision forwards the optional UnitDecisionReader capability. A store
// without it holds no ledger to read, which reads as "no decision yet" — the
// same answer a unit awaiting its first decision gives.
func (s *EventEmittingStore) GetUnitDecision(ctx context.Context, projectID, stream, itemName, unit, variant string) (*venue.UnitDecision, error) {
	dr, ok := s.inner.(store.UnitDecisionReader)
	if !ok {
		return nil, nil
	}
	return dr.GetUnitDecision(ctx, projectID, stream, itemName, unit, variant)
}

// TallyDecisionBasis forwards the optional DecisionStore capability. A store
// that keeps no ledger grades nothing, which the dashboard reads as no stale
// units — the same answer a project with no decisions gives.
func (s *EventEmittingStore) TallyDecisionBasis(ctx context.Context, projectID, stream string) ([]store.DecisionBasisTally, error) {
	ds, ok := s.inner.(store.DecisionStore)
	if !ok {
		return nil, nil
	}
	return ds.TallyDecisionBasis(ctx, projectID, stream)
}

// GetBlockAccess forwards the optional BlockAccessStore capability.
func (s *EventEmittingStore) GetBlockAccess(ctx context.Context, projectID, stream, blockID string) (string, string, error) {
	as, ok := s.inner.(store.BlockAccessStore)
	if !ok {
		return "", "", fmt.Errorf("content store %T keeps no access ladder", s.inner)
	}
	return as.GetBlockAccess(ctx, projectID, stream, blockID)
}

// SetBlockAccess forwards the optional BlockAccessStore capability.
func (s *EventEmittingStore) SetBlockAccess(ctx context.Context, projectID, stream, blockID, access, ownerID string) error {
	as, ok := s.inner.(store.BlockAccessStore)
	if !ok {
		return fmt.Errorf("content store %T keeps no access ladder", s.inner)
	}
	return as.SetBlockAccess(ctx, projectID, stream, blockID, access, ownerID)
}

// GetLastEditor forwards the optional BlockAccessStore capability.
func (s *EventEmittingStore) GetLastEditor(ctx context.Context, projectID, stream, blockID string) (string, error) {
	as, ok := s.inner.(store.BlockAccessStore)
	if !ok {
		return "", fmt.Errorf("content store %T keeps no access ladder", s.inner)
	}
	return as.GetLastEditor(ctx, projectID, stream, blockID)
}

// LastTargetAuthors forwards the optional TargetAuthorStore capability. The
// server holds this wrapper, so separation of duties at review reaches the
// underlying store through here or not at all.
func (s *EventEmittingStore) LastTargetAuthors(ctx context.Context, projectID, stream string, blockIDs, locales []string) (map[store.TargetRef]string, error) {
	as, ok := s.inner.(store.TargetAuthorStore)
	if !ok {
		return nil, fmt.Errorf("content store %T keeps no target authorship", s.inner)
	}
	return as.LastTargetAuthors(ctx, projectID, stream, blockIDs, locales)
}

// UpsertChannelAliasProposals forwards the optional ChannelAliasStore
// capability. Same reason as the decision ledger above: the server holds the
// wrapper, and an assertion against the wrapper's method set found nothing —
// so the push raised no proposals and the listing answered "store not
// configured", on a server whose store keeps the table perfectly well.
func (s *EventEmittingStore) UpsertChannelAliasProposals(ctx context.Context, proposals []store.ChannelAliasProposal) (int, error) {
	as, ok := s.inner.(store.ChannelAliasStore)
	if !ok {
		return 0, fmt.Errorf("content store %T keeps no channel alias proposals", s.inner)
	}
	return as.UpsertChannelAliasProposals(ctx, proposals)
}

// ListChannelAliasProposals forwards the optional ChannelAliasStore capability.
func (s *EventEmittingStore) ListChannelAliasProposals(ctx context.Context, workspaceID, status string) ([]store.ChannelAliasProposal, error) {
	as, ok := s.inner.(store.ChannelAliasStore)
	if !ok {
		return nil, fmt.Errorf("content store %T keeps no channel alias proposals", s.inner)
	}
	return as.ListChannelAliasProposals(ctx, workspaceID, status)
}

// JudgeChannelAliasProposal forwards the optional ChannelAliasStore capability.
func (s *EventEmittingStore) JudgeChannelAliasProposal(ctx context.Context, j store.ChannelAliasJudgement) (bool, error) {
	as, ok := s.inner.(store.ChannelAliasStore)
	if !ok {
		return false, fmt.Errorf("content store %T keeps no channel alias proposals", s.inner)
	}
	return as.JudgeChannelAliasProposal(ctx, j)
}
