package service

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

// ProjectService provides project and block operations.
type ProjectService struct {
	store store.ContentStore
}

// NewProjectService creates a new ProjectService.
func NewProjectService(s store.ContentStore) *ProjectService {
	return &ProjectService{store: s}
}

// CreateProject creates a new project.
func (s *ProjectService) CreateProject(ctx context.Context, p *store.Project) error {
	if p == nil {
		return fmt.Errorf("project is required")
	}
	if p.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if p.DefaultSourceLanguage == "" {
		return fmt.Errorf("source locale is required")
	}
	return s.store.CreateProject(ctx, p)
}

// GetProject retrieves a project by ID.
func (s *ProjectService) GetProject(ctx context.Context, id string) (*store.Project, error) {
	if id == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	return s.store.GetProject(ctx, id)
}

// ListProjects returns all projects.
func (s *ProjectService) ListProjects(ctx context.Context) ([]*store.Project, error) {
	return s.store.ListProjects(ctx)
}

// UpdateProject updates a project.
func (s *ProjectService) UpdateProject(ctx context.Context, p *store.Project) error {
	if p == nil {
		return fmt.Errorf("project is required")
	}
	if p.ID == "" {
		return fmt.Errorf("project ID is required")
	}
	return s.store.UpdateProject(ctx, p)
}

// DeleteProject deletes a project.
func (s *ProjectService) DeleteProject(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("project ID is required")
	}
	return s.store.DeleteProject(ctx, id)
}

// StoreBlocks stores blocks in a project.
func (s *ProjectService) StoreBlocks(ctx context.Context, projectID string, blocks []*model.Block) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}
	return s.store.StoreBlocks(ctx, projectID, "main", blocks)
}

// StoreBlocksForItem stores blocks scoped to a specific item (source file).
func (s *ProjectService) StoreBlocksForItem(ctx context.Context, projectID, itemName string, blocks []*model.Block) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}
	if itemName == "" {
		return fmt.Errorf("item name is required")
	}
	return s.store.StoreBlocksForItem(ctx, projectID, "main", itemName, blocks)
}

// GetBlock retrieves a single block.
func (s *ProjectService) GetBlock(ctx context.Context, projectID, blockID string) (*store.StoredBlock, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	if blockID == "" {
		return nil, fmt.Errorf("block ID is required")
	}
	return s.store.GetBlock(ctx, projectID, "main", blockID)
}

// GetBlocks retrieves blocks matching a query.
func (s *ProjectService) GetBlocks(ctx context.Context, query store.BlockQuery) ([]*store.StoredBlock, error) {
	if query.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required in block query")
	}
	if query.Stream == "" {
		query.Stream = "main"
	}
	return s.store.GetBlocks(ctx, query)
}

// CreateVersion creates a version snapshot.
func (s *ProjectService) CreateVersion(ctx context.Context, projectID, label, description string) (*store.Version, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	if label == "" {
		return nil, fmt.Errorf("version label is required")
	}
	return s.store.CreateVersion(ctx, projectID, "main", label, description)
}

// ListVersions lists all versions for a project.
func (s *ProjectService) ListVersions(ctx context.Context, projectID string) ([]*store.Version, error) {
	return s.store.ListVersions(ctx, projectID, "main")
}

// Diff computes the diff between two versions.
func (s *ProjectService) Diff(ctx context.Context, from, to string) (*store.VersionDiff, error) {
	return s.store.Diff(ctx, from, to)
}

// GetChanges returns change log entries since the given cursor.
func (s *ProjectService) GetChanges(ctx context.Context, projectID string, sinceCursor int64, locales []string, limit int) (*store.ChangeSet, error) {
	return s.store.GetChanges(ctx, projectID, "main", sinceCursor, locales, limit)
}

// LatestCursor returns the most recent change log sequence number for a project.
func (s *ProjectService) LatestCursor(ctx context.Context, projectID string) (int64, error) {
	return s.store.LatestCursor(ctx, projectID, "main")
}

// DeleteBlock deletes a block from a project.
func (s *ProjectService) DeleteBlock(ctx context.Context, projectID, blockID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}
	if blockID == "" {
		return fmt.Errorf("block ID is required")
	}
	return s.store.DeleteBlock(ctx, projectID, "main", blockID)
}
