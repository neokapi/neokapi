package service

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/connector"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/store"
)

// ConnectorService manages connectors and orchestrates pull/push operations.
type ConnectorService struct {
	store        store.ContentStore
	connectorReg *connector.Registry
	active       map[string]connector.Connector // connectorID → instance
}

// NewConnectorService creates a new ConnectorService.
func NewConnectorService(s store.ContentStore, reg *connector.Registry) *ConnectorService {
	return &ConnectorService{
		store:        s,
		connectorReg: reg,
		active:       make(map[string]connector.Connector),
	}
}

// ListConnectorTypes returns available connector types.
func (s *ConnectorService) ListConnectorTypes() []connector.ConnectorInfo {
	return s.connectorReg.List()
}

// AddConnector creates and registers an active connector instance.
func (s *ConnectorService) AddConnector(name string, config map[string]string) (connector.Connector, error) {
	c, err := s.connectorReg.NewConnector(name, config)
	if err != nil {
		return nil, fmt.Errorf("create connector %s: %w", name, err)
	}
	s.active[c.ID()] = c
	return c, nil
}

// GetConnector returns an active connector by ID.
func (s *ConnectorService) GetConnector(id string) (connector.Connector, error) {
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
func (s *ConnectorService) ListActive() []connector.Connector {
	result := make([]connector.Connector, 0, len(s.active))
	for _, c := range s.active {
		result = append(result, c)
	}
	return result
}

// Pull retrieves content from a connector and stores it in the project.
func (s *ConnectorService) Pull(ctx context.Context, connectorID, projectID string, opts connector.PullOptions) ([]*connector.ContentItem, error) {
	c, ok := s.active[connectorID]
	if !ok {
		return nil, fmt.Errorf("connector %s not found", connectorID)
	}

	items, err := c.Pull(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("pull from %s: %w", connectorID, err)
	}

	// Store pulled blocks in the content store.
	for _, item := range items {
		if len(item.Blocks) > 0 {
			if err := s.store.StoreBlocks(ctx, projectID, item.Blocks); err != nil {
				return nil, fmt.Errorf("store blocks from %s: %w", item.Path, err)
			}
		}
	}

	return items, nil
}

// Push sends content from the store to a connector.
func (s *ConnectorService) Push(ctx context.Context, connectorID, projectID string, opts connector.PushOptions) error {
	c, ok := s.active[connectorID]
	if !ok {
		return fmt.Errorf("connector %s not found", connectorID)
	}

	// Get all blocks from the project.
	blocks, err := s.store.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID})
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	// Group blocks into a single content item for pushing.
	modelBlocks := make([]*model.Block, len(blocks))
	for i, sb := range blocks {
		modelBlocks[i] = sb.Block
	}

	items := []*connector.ContentItem{{
		ID:     projectID,
		Blocks: modelBlocks,
	}}

	return c.Push(ctx, items, opts)
}

// SyncStatus returns the sync status for a connector.
func (s *ConnectorService) SyncStatus(ctx context.Context, connectorID string) (*connector.SyncStatus, error) {
	c, ok := s.active[connectorID]
	if !ok {
		return nil, fmt.Errorf("connector %s not found", connectorID)
	}
	return c.Sync(ctx)
}
