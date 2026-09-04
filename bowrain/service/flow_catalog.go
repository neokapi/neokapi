package service

import (
	"context"
	"errors"
	"fmt"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/host/flowdef"
)

// ErrFlowNotFound reports a flow id that neither the built-in catalog nor the
// project's stored definitions carry. It wraps the store's own not-found error
// so a caller checking either sentinel sees the miss.
var ErrFlowNotFound = fmt.Errorf("flow not found: %w", bstore.ErrFlowDefNotFound)

// FlowCatalog answers which flow definitions a project can run: the built-in
// catalog (flowdef.BuiltInFlows) merged with the project's stored definitions
// (Bowrain AD-013). Every surface that names a flow by id, the REST flow
// routes, an automation rule's run_flow action, and the MCP run_flow tool,
// resolves through it, so the same id means the same flow everywhere.
//
// A nil store limits the catalog to the built-in flows.
type FlowCatalog struct {
	defs *bstore.FlowDefStore
}

// NewFlowCatalog builds a catalog over the project flow store. Pass nil when
// no store is configured.
func NewFlowCatalog(defs *bstore.FlowDefStore) *FlowCatalog {
	return &FlowCatalog{defs: defs}
}

// List returns the built-in flows followed by the project's stored flows,
// which is the order the flow picker shows them in.
func (c *FlowCatalog) List(ctx context.Context, projectID string) ([]flow.FlowDefinition, error) {
	defs := make([]flow.FlowDefinition, 0, 16)
	defs = append(defs, flowdef.BuiltInFlows()...)
	if c == nil || c.defs == nil {
		return defs, nil
	}
	stored, err := c.defs.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return append(defs, stored...), nil
}

// Get resolves one flow by id for a project. A built-in id wins; a stored
// definition is looked up under the given project only, so a flow another
// project authored resolves to ErrFlowNotFound here.
func (c *FlowCatalog) Get(ctx context.Context, projectID, flowID string) (*flow.FlowDefinition, error) {
	if flowID == "" {
		return nil, fmt.Errorf("%w: empty flow id", ErrFlowNotFound)
	}
	for _, def := range flowdef.BuiltInFlows() {
		if def.ID == flowID {
			d := def
			return &d, nil
		}
	}
	if c == nil || c.defs == nil {
		return nil, fmt.Errorf("%w: %q", ErrFlowNotFound, flowID)
	}
	def, err := c.defs.Get(ctx, projectID, flowID)
	if errors.Is(err, bstore.ErrFlowDefNotFound) {
		return nil, fmt.Errorf("%w: %q", ErrFlowNotFound, flowID)
	}
	if err != nil {
		return nil, err
	}
	return def, nil
}

// IsBuiltInFlow reports whether id names a flow from the built-in catalog.
func IsBuiltInFlow(id string) bool {
	for _, def := range flowdef.BuiltInFlows() {
		if def.ID == id {
			return true
		}
	}
	return false
}
