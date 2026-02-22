package service

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/platform/store"
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
func (s *FlowService) ExecuteFlow(ctx context.Context, f *flow.Flow, items []*flow.FlowItem, projectID string) error {
	opts := []flow.ExecutorOption{
		flow.WithFailFast(true),
	}

	executor := flow.NewFlowExecutor(opts...)
	if err := executor.Execute(ctx, f, items); err != nil {
		return fmt.Errorf("execute flow: %w", err)
	}

	return nil
}
