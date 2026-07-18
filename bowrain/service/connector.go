package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// ConnectorService manages connectors and orchestrates fetch/publish operations.
//
// Active connector instances are scoped per workspace: a connector added in one
// workspace cannot be addressed (fetched/published/status/removed) from another,
// even though connector IDs are derived from connection settings and are thus
// guessable. A standalone/single-user server uses the "" workspace key.
type ConnectorService struct {
	store        store.ContentStore
	connectorReg *connector.Registry

	mu sync.Mutex
	// active maps workspaceID -> connectorID -> instance.
	active map[string]map[string]connector.IntegrationConnector

	// tracker captures connector_published analytics events. Optional; nil
	// disables capture (see Services.SetEventTracker).
	tracker EventTracker
}

// NewConnectorService creates a new ConnectorService.
func NewConnectorService(s store.ContentStore, reg *connector.Registry) *ConnectorService {
	return &ConnectorService{
		store:        s,
		connectorReg: reg,
		active:       make(map[string]map[string]connector.IntegrationConnector),
	}
}

// ListConnectorTypes returns available connector types.
func (s *ConnectorService) ListConnectorTypes() []connector.Info {
	return s.connectorReg.List()
}

// AddConnector creates and registers an active connector instance in a workspace.
func (s *ConnectorService) AddConnector(workspaceID, name string, config map[string]string) (connector.IntegrationConnector, error) {
	if name == "" {
		return nil, ErrConnectorNameRequired
	}
	c, err := s.connectorReg.NewConnector(name, config)
	if err != nil {
		return nil, fmt.Errorf("create connector %s: %w", name, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ws := s.active[workspaceID]
	if ws == nil {
		ws = make(map[string]connector.IntegrationConnector)
		s.active[workspaceID] = ws
	}
	ws[c.ID()] = c
	return c, nil
}

// lookup returns an active connector within a workspace, or ErrConnectorNotFound.
// Connector returns the live connector instance for a workspace, for callers
// that need the concrete connector surface (e.g. forge delivery drives
// Publish with per-item target files rather than the generic single-item
// grouping of ConnectorService.Publish).
func (s *ConnectorService) Connector(workspaceID, id string) (connector.IntegrationConnector, error) {
	return s.lookup(workspaceID, id)
}

func (s *ConnectorService) lookup(workspaceID, id string) (connector.IntegrationConnector, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.active[workspaceID][id]
	if !ok {
		return nil, fmt.Errorf("connector %s: %w", id, ErrConnectorNotFound)
	}
	return c, nil
}

// GetConnector returns an active connector by workspace + ID.
func (s *ConnectorService) GetConnector(workspaceID, id string) (connector.IntegrationConnector, error) {
	return s.lookup(workspaceID, id)
}

// RemoveConnector closes and removes an active connector within a workspace.
func (s *ConnectorService) RemoveConnector(workspaceID, id string) error {
	s.mu.Lock()
	c, ok := s.active[workspaceID][id]
	if ok {
		delete(s.active[workspaceID], id)
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("connector %s: %w", id, ErrConnectorNotFound)
	}
	return c.Close()
}

// ListActive returns all active connector instances in a workspace.
func (s *ConnectorService) ListActive(workspaceID string) []connector.IntegrationConnector {
	s.mu.Lock()
	defer s.mu.Unlock()
	ws := s.active[workspaceID]
	result := make([]connector.IntegrationConnector, 0, len(ws))
	for _, c := range ws {
		result = append(result, c)
	}
	return result
}

// ListContent returns the content items available from a connector without
// fetching full content into the store. It mirrors the desktop app's in-process
// ListContentItems binding (a plain c.List): read-only, connector-wide, and
// project-agnostic. Callers may narrow the result by path/ID after the fact.
func (s *ConnectorService) ListContent(ctx context.Context, workspaceID, connectorID string) ([]*connector.ContentItem, error) {
	c, err := s.lookup(workspaceID, connectorID)
	if err != nil {
		return nil, err
	}
	return c.List(ctx)
}

// Fetch retrieves content from a connector and stores it in the project —
// per item, with the item bookkeeping every other ingest venue writes.
//
// Each fetched ContentItem upserts an item row and stores its blocks scoped to
// that item (StoreBlocksForItem), matching the editor-upload, sync-push, and
// desktop ingest paths. The item rows are load-bearing, not cosmetic: the
// dashboard stats (GetBlockStats), the convergence derive/produce steps (jobs
// are enqueued per item), and forge delivery (per-item materialization) all
// resolve a project's content through its items — blocks stored item-less were
// invisible to them, so a freshly bound GitHub repository read "Up to date"
// with zero targets and delivered nothing. Per-item storage also keeps
// format-reader block IDs (per-file scope) from colliding across files: the
// store maps each item's source IDs to project-unique internal IDs.
//
// Each item stores in its own transaction; re-fetching is idempotent (items
// upsert by name, blocks by their per-item source IDs), so a partial failure
// heals on the next ingest.
func (s *ConnectorService) Fetch(ctx context.Context, workspaceID, connectorID, projectID string, opts connector.FetchOptions) ([]*connector.ContentItem, error) {
	if projectID == "" {
		return nil, ErrProjectIDRequired
	}
	c, err := s.lookup(workspaceID, connectorID)
	if err != nil {
		return nil, err
	}

	items, err := c.Fetch(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("fetch from %s: %w", connectorID, err)
	}

	for _, item := range items {
		name := connectorItemName(item)
		if name == "" || len(item.Blocks) == 0 {
			continue // nothing addressable or nothing translatable to store
		}
		if err := s.store.StoreItem(ctx, projectID, "main", &store.Item{
			Name:       name,
			Format:     item.Format,
			ItemType:   "file",
			Properties: map[string]string{},
		}); err != nil {
			return nil, fmt.Errorf("store fetched item %q: %w", name, err)
		}
		if err := s.store.StoreBlocksForItem(ctx, projectID, "main", name, item.Blocks); err != nil {
			return nil, fmt.Errorf("store fetched blocks for %q: %w", name, err)
		}
	}

	return items, nil
}

// connectorItemName resolves the store item name for a fetched ContentItem:
// the source-relative Path when the connector reports one (file/git/forge —
// unique, and what delivery derives target paths from), else the item's Name,
// else its ID (CMS connectors address content by ID).
func connectorItemName(item *connector.ContentItem) string {
	if item == nil {
		return ""
	}
	if item.Path != "" {
		return item.Path
	}
	if item.Name != "" {
		return item.Name
	}
	return item.ID
}

// Publish sends content from the store to a connector.
func (s *ConnectorService) Publish(ctx context.Context, workspaceID, connectorID, projectID string, opts connector.PublishOptions) error {
	if projectID == "" {
		return ErrProjectIDRequired
	}
	c, err := s.lookup(workspaceID, connectorID)
	if err != nil {
		return err
	}

	// Get all blocks from the project.
	blocks, err := s.store.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main"})
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	// Group blocks into a single content item for publishing.
	modelBlocks := make([]*model.Block, len(blocks))
	for i, sb := range blocks {
		modelBlocks[i] = sb.Block
	}

	items := []*connector.ContentItem{{
		ID:     projectID,
		Blocks: modelBlocks,
	}}

	err = c.Publish(ctx, items, opts)

	// Fire-and-forget analytics after the publish attempt resolves; never
	// blocks or alters the result (epic 018 workstream D).
	outcome := "completed"
	if err != nil {
		outcome = "failed"
	}
	props := analytics.Props(workspaceID, projectID)
	props["connector_type"] = c.Name()
	props["outcome"] = outcome
	props["block_count_bucket"] = analytics.CountBucket(len(modelBlocks))
	track(s.tracker, workspaceID, analytics.EventConnectorPublished, props)

	return err
}

// ConnectorStatus returns the sync status for a connector.
func (s *ConnectorService) ConnectorStatus(ctx context.Context, workspaceID, connectorID string) (*connector.SyncStatus, error) {
	c, err := s.lookup(workspaceID, connectorID)
	if err != nil {
		return nil, err
	}
	return c.Status(ctx)
}
