package service

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/platform/connector"
	"github.com/gokapi/gokapi/platform/store"
)

// ConnectorService manages connectors and orchestrates fetch/publish operations.
type ConnectorService struct {
	store        store.ContentStore
	connectorReg *connector.Registry
	active       map[string]connector.IntegrationConnector // connectorID -> instance
}

// NewConnectorService creates a new ConnectorService.
func NewConnectorService(s store.ContentStore, reg *connector.Registry) *ConnectorService {
	return &ConnectorService{
		store:        s,
		connectorReg: reg,
		active:       make(map[string]connector.IntegrationConnector),
	}
}

// ListConnectorTypes returns available connector types.
func (s *ConnectorService) ListConnectorTypes() []connector.ConnectorInfo {
	return s.connectorReg.List()
}

// AddConnector creates and registers an active connector instance.
func (s *ConnectorService) AddConnector(name string, config map[string]string) (connector.IntegrationConnector, error) {
	if name == "" {
		return nil, fmt.Errorf("connector name is required")
	}
	c, err := s.connectorReg.NewConnector(name, config)
	if err != nil {
		return nil, fmt.Errorf("create connector %s: %w", name, err)
	}
	s.active[c.ID()] = c
	return c, nil
}

// GetConnector returns an active connector by ID.
func (s *ConnectorService) GetConnector(id string) (connector.IntegrationConnector, error) {
	c, ok := s.active[id]
	if !ok {
		return nil, fmt.Errorf("connector %s not found", id)
	}
	return c, nil
}

// RemoveConnector closes and removes an active connector.
func (s *ConnectorService) RemoveConnector(id string) error {
	c, ok := s.active[id]
	if !ok {
		return fmt.Errorf("connector %s not found", id)
	}
	delete(s.active, id)
	return c.Close()
}

// ListActive returns all active connector instances.
func (s *ConnectorService) ListActive() []connector.IntegrationConnector {
	result := make([]connector.IntegrationConnector, 0, len(s.active))
	for _, c := range s.active {
		result = append(result, c)
	}
	return result
}

// Fetch retrieves content from a connector and stores it in the project.
func (s *ConnectorService) Fetch(ctx context.Context, connectorID, projectID string, opts connector.FetchOptions) ([]*connector.ContentItem, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	c, ok := s.active[connectorID]
	if !ok {
		return nil, fmt.Errorf("connector %s not found", connectorID)
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
		if err := s.store.StoreBlocks(ctx, projectID, allBlocks); err != nil {
			return nil, fmt.Errorf("store fetched blocks: %w", err)
		}
	}

	return items, nil
}

// Publish sends content from the store to a connector.
func (s *ConnectorService) Publish(ctx context.Context, connectorID, projectID string, opts connector.PublishOptions) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}
	c, ok := s.active[connectorID]
	if !ok {
		return fmt.Errorf("connector %s not found", connectorID)
	}

	// Get all blocks from the project.
	blocks, err := s.store.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID})
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
func (s *ConnectorService) ConnectorStatus(ctx context.Context, connectorID string) (*connector.SyncStatus, error) {
	c, ok := s.active[connectorID]
	if !ok {
		return nil, fmt.Errorf("connector %s not found", connectorID)
	}
	return c.Status(ctx)
}
