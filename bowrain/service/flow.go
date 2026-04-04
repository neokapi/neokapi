package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
)

// FlowService manages flow execution with optional store integration.
type FlowService struct {
	store     store.ContentStore
	formatReg *registry.FormatRegistry
	toolReg   *registry.ToolRegistry
}

// NewFlowService creates a new FlowService.
func NewFlowService(s store.ContentStore, formatReg *registry.FormatRegistry, toolReg *registry.ToolRegistry) *FlowService {
	return &FlowService{
		store:     s,
		formatReg: formatReg,
		toolReg:   toolReg,
	}
}

// ExecuteFlow runs a flow definition with optional store-backed persistence.
// When projectID is non-empty and a content store is configured, output blocks
// are persisted to the store after successful execution.
func (s *FlowService) ExecuteFlow(ctx context.Context, f *flow.Flow, items []*flow.Item, projectID string) error {
	if f == nil {
		return errors.New("flow definition is required")
	}
	if len(items) == 0 {
		return errors.New("at least one flow item is required")
	}
	opts := []flow.ExecutorOption{
		flow.WithFailFast(true),
	}

	executor := flow.NewExecutor(opts...)
	if err := executor.Execute(ctx, f, items); err != nil {
		return fmt.Errorf("execute flow: %w", err)
	}

	// Persist output blocks to the content store if project-scoped.
	if projectID != "" && s.store != nil {
		for _, item := range items {
			if len(item.OutputBlocks) > 0 {
				if err := s.store.StoreBlocks(ctx, projectID, "main", item.OutputBlocks); err != nil {
					return fmt.Errorf("persist flow output blocks: %w", err)
				}
			}
		}
	}

	return nil
}
