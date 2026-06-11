package backend

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
)

// ConnectorInfo describes a connector for the frontend.
type ConnectorInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Category string `json:"category"`
}

// ContentItemInfo describes a content item fetched from a connector.
type ContentItemInfo struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Title      string `json:"title"`
	BlockCount int    `json:"block_count"`
}

// SyncStatusInfo describes the sync status of a connector.
type SyncStatusInfo struct {
	ConnectorID string `json:"connector_id"`
	LastSync    string `json:"last_sync,omitempty"`
	ItemCount   int    `json:"item_count"`
	Status      string `json:"status"`
}

// activeConnectors holds instantiated connectors by ID.
var activeConnectors = map[string]connector.IntegrationConnector{}

// ListConnectorTypes returns all available connector type names.
func (a *App) ListConnectorTypes() []string {
	infos := a.connectorReg.List()
	names := make([]string, len(infos))
	for i, info := range infos {
		names[i] = info.Name
	}
	return names
}

// ListConnectors returns all active connector instances.
func (a *App) ListConnectors() []ConnectorInfo {
	var result []ConnectorInfo
	for _, c := range activeConnectors {
		result = append(result, ConnectorInfo{
			ID:       c.ID(),
			Name:     c.Name(),
			Type:     c.Name(),
			Category: string(c.Category()),
		})
	}
	return result
}

// ConfigureConnector creates and stores a connector instance.
func (a *App) ConfigureConnector(connectorType string, config map[string]string) (*ConnectorInfo, error) {
	c, err := a.connectorReg.NewConnector(connectorType, config)
	if err != nil {
		return nil, fmt.Errorf("create connector %s: %w", connectorType, err)
	}
	activeConnectors[c.ID()] = c
	return &ConnectorInfo{
		ID:       c.ID(),
		Name:     c.Name(),
		Type:     connectorType,
		Category: string(c.Category()),
	}, nil
}

// RemoveConnector removes an active connector.
func (a *App) RemoveConnector(connectorID string) error {
	c, ok := activeConnectors[connectorID]
	if !ok {
		return fmt.Errorf("connector %s not found", connectorID)
	}
	delete(activeConnectors, connectorID)
	return c.Close()
}

// FetchContent fetches content from a connector into the content store.
func (a *App) FetchContent(connectorID, projectID string) ([]ContentItemInfo, error) {
	c, ok := activeConnectors[connectorID]
	if !ok {
		return nil, fmt.Errorf("connector %s not found", connectorID)
	}

	ctx := context.Background()
	items, err := c.Fetch(ctx, connector.FetchOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetch from %s: %w", connectorID, err)
	}

	// Store blocks if we have a content store.
	if a.store != nil && projectID != "" {
		for _, item := range items {
			if len(item.Blocks) > 0 {
				if err := a.store.StoreBlocks(ctx, projectID, "main", item.Blocks); err != nil {
					return nil, fmt.Errorf("store blocks: %w", err)
				}
			}
		}
	}

	var result []ContentItemInfo
	for _, item := range items {
		result = append(result, ContentItemInfo{
			ID:         item.ID,
			Path:       item.Path,
			Title:      item.Name,
			BlockCount: len(item.Blocks),
		})
	}
	return result, nil
}

// PublishContent publishes content from the store to a connector.
func (a *App) PublishContent(connectorID, projectID string) error {
	c, ok := activeConnectors[connectorID]
	if !ok {
		return fmt.Errorf("connector %s not found", connectorID)
	}
	if a.store == nil {
		return errors.New("content store not initialized")
	}

	ctx := context.Background()
	blocks, err := a.store.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main"})
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	modelBlocks := make([]*model.Block, len(blocks))
	for i, sb := range blocks {
		modelBlocks[i] = sb.Block
	}

	items := []*connector.ContentItem{{
		ID:     projectID,
		Blocks: modelBlocks,
	}}

	return c.Publish(ctx, items, connector.PublishOptions{})
}

// GetConnectorStatus returns the sync status of a connector.
func (a *App) GetConnectorStatus(connectorID string) (*SyncStatusInfo, error) {
	c, ok := activeConnectors[connectorID]
	if !ok {
		return nil, fmt.Errorf("connector %s not found", connectorID)
	}

	ctx := context.Background()
	status, err := c.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("connector status: %w", err)
	}

	statusStr := "synced"
	if len(status.Errors) > 0 {
		statusStr = "error"
	} else if status.PendingPull > 0 || status.PendingPush > 0 {
		statusStr = "pending"
	}
	return &SyncStatusInfo{
		ConnectorID: connectorID,
		LastSync:    status.LastSync.Format("2006-01-02T15:04:05Z07:00"),
		ItemCount:   status.ItemCount,
		Status:      statusStr,
	}, nil
}

// ListContentItems lists content available from a connector.
func (a *App) ListContentItems(connectorID string) ([]ContentItemInfo, error) {
	c, ok := activeConnectors[connectorID]
	if !ok {
		return nil, fmt.Errorf("connector %s not found", connectorID)
	}

	ctx := context.Background()
	items, err := c.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}

	var result []ContentItemInfo
	for _, item := range items {
		result = append(result, ContentItemInfo{
			ID:         item.ID,
			Path:       item.Path,
			Title:      item.Name,
			BlockCount: len(item.Blocks),
		})
	}
	return result, nil
}

// InitContentStore re-initializes the content store with the given database path.
func (a *App) InitContentStore(dbPath string) error {
	if a.store != nil {
		a.store.Close()
	}
	cs, err := bstore.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("open content store: %w", err)
	}
	a.store = cs
	return nil
}

// StoreProject creates a project in the content store.
func (a *App) StoreProject(name, sourceLocale string, targetLocales []string) (*store.Project, error) {
	locales := make([]model.LocaleID, len(targetLocales))
	for i, l := range targetLocales {
		locales[i] = model.LocaleID(l)
	}
	p := &store.Project{
		Name:                  name,
		DefaultSourceLanguage: model.LocaleID(sourceLocale),
		TargetLanguages:       locales,
		Properties:            map[string]string{},
	}
	if err := a.store.CreateProject(context.Background(), p); err != nil {
		return nil, err
	}
	return p, nil
}

// ListStoreProjects returns all projects in the content store.
func (a *App) ListStoreProjects() ([]*store.Project, error) {
	return a.store.ListProjects(context.Background())
}

// CreateStoreVersion creates a version snapshot in the content store.
func (a *App) CreateStoreVersion(projectID, label, description string) (*store.Version, error) {
	return a.store.CreateVersion(context.Background(), projectID, "main", label, description)
}

// ListStoreVersions lists versions for a project.
func (a *App) ListStoreVersions(projectID string) ([]*store.Version, error) {
	return a.store.ListVersions(context.Background(), projectID, "main")
}
