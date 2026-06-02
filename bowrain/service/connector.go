package service

import (
	"context"
	"fmt"
	"sync"

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

// Fetch retrieves content from a connector and stores it in the project.
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

	// Collect all blocks and store them in a single call so the underlying
	// transaction is atomic — either all blocks are persisted or none are.
	var allBlocks []*model.Block
	for _, item := range items {
		allBlocks = append(allBlocks, item.Blocks...)
	}
	if len(allBlocks) > 0 {
		if err := s.store.StoreBlocks(ctx, projectID, "main", allBlocks); err != nil {
			return nil, fmt.Errorf("store fetched blocks: %w", err)
		}
	}

	return items, nil
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

	return c.Publish(ctx, items, opts)
}

// ConnectorStatus returns the sync status for a connector.
func (s *ConnectorService) ConnectorStatus(ctx context.Context, workspaceID, connectorID string) (*connector.SyncStatus, error) {
	c, err := s.lookup(workspaceID, connectorID)
	if err != nil {
		return nil, err
	}
	return c.Status(ctx)
}
