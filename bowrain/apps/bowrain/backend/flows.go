package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
)

// FlowDefinitionInfo is the frontend-facing flow definition type.
type FlowDefinitionInfo struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Nodes       []FlowNodeInfo `json:"nodes"`
	Edges       []FlowEdgeInfo `json:"edges"`
	Source      string         `json:"source"`
	CreatedAt   string         `json:"created_at,omitempty"`
	ModifiedAt  string         `json:"modified_at,omitempty"`
}

// FlowNodeInfo is the frontend-facing flow node type.
type FlowNodeInfo struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Label    string         `json:"label,omitempty"`
	// Stage is the pipeline stage for this node. Empty means the main stage;
	// "source-transform" means the leading source-rewrite stage.
	Stage    string         `json:"stage,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Position PositionInfo   `json:"position"`
}

// PositionInfo holds x/y for a node.
type PositionInfo struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// FlowEdgeInfo is the frontend-facing flow edge type.
type FlowEdgeInfo struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func flowDefToInfo(def flow.FlowDefinition) FlowDefinitionInfo {
	nodes := make([]FlowNodeInfo, len(def.Nodes))
	for i, n := range def.Nodes {
		nodes[i] = FlowNodeInfo{
			ID:       n.ID,
			Type:     string(n.Type),
			Name:     n.Name,
			Label:    n.Label,
			Stage:    string(n.Stage),
			Config:   n.Config,
			Position: PositionInfo{X: n.Position.X, Y: n.Position.Y},
		}
	}
	edges := make([]FlowEdgeInfo, len(def.Edges))
	for i, e := range def.Edges {
		edges[i] = FlowEdgeInfo{
			ID:     e.ID,
			Source: e.Source,
			Target: e.Target,
		}
	}
	return FlowDefinitionInfo{
		ID:          def.ID,
		Name:        def.Name,
		Description: def.Description,
		Nodes:       nodes,
		Edges:       edges,
		Source:      def.Source,
		CreatedAt:   def.CreatedAt,
		ModifiedAt:  def.ModifiedAt,
	}
}

func infoToFlowDef(info FlowDefinitionInfo) flow.FlowDefinition {
	nodes := make([]flow.FlowNode, len(info.Nodes))
	for i, n := range info.Nodes {
		nodes[i] = flow.FlowNode{
			ID:       n.ID,
			Type:     flow.NodeType(n.Type),
			Name:     n.Name,
			Label:    n.Label,
			Stage:    flow.FlowStage(n.Stage),
			Config:   n.Config,
			Position: flow.NodePosition{X: n.Position.X, Y: n.Position.Y},
		}
	}
	edges := make([]flow.FlowEdge, len(info.Edges))
	for i, e := range info.Edges {
		edges[i] = flow.FlowEdge{
			ID:     e.ID,
			Source: e.Source,
			Target: e.Target,
		}
	}
	return flow.FlowDefinition{
		ID:          info.ID,
		Name:        info.Name,
		Description: info.Description,
		Nodes:       nodes,
		Edges:       edges,
		Source:      info.Source,
		CreatedAt:   info.CreatedAt,
		ModifiedAt:  info.ModifiedAt,
	}
}

func (a *App) flowStore() *flow.FlowStore {
	dir := filepath.Join(defaultBowrainDir(), "flows")
	return flow.NewFlowStore(dir)
}

func defaultBowrainDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".bowrain")
	}
	return ".bowrain"
}

// ListFlowDefinitions returns all flow definitions (built-in + user).
func (a *App) ListFlowDefinitions() []FlowDefinitionInfo {
	var result []FlowDefinitionInfo

	// Built-in flows.
	for _, def := range flow.BuiltInFlows() {
		result = append(result, flowDefToInfo(def))
	}

	// User flows.
	store := a.flowStore()
	userDefs, err := store.List()
	if err == nil {
		for _, def := range userDefs {
			result = append(result, flowDefToInfo(def))
		}
	}

	return result
}

// GetFlowDefinition returns a specific flow definition by ID.
func (a *App) GetFlowDefinition(id string) (*FlowDefinitionInfo, error) {
	// Check built-in flows first.
	for _, def := range flow.BuiltInFlows() {
		if def.ID == id {
			info := flowDefToInfo(def)
			return &info, nil
		}
	}

	// Check user flows.
	store := a.flowStore()
	def, err := store.Get(id)
	if err != nil {
		return nil, fmt.Errorf("flow %q not found", id)
	}
	info := flowDefToInfo(*def)
	return &info, nil
}

// SaveFlowDefinition saves a user flow definition.
func (a *App) SaveFlowDefinition(info FlowDefinitionInfo) (*FlowDefinitionInfo, error) {
	if info.Source == registry.SourceBuiltIn {
		return nil, fmt.Errorf("cannot modify built-in flows")
	}

	def := infoToFlowDef(info)
	now := time.Now().UTC().Format(time.RFC3339)
	if def.CreatedAt == "" {
		def.CreatedAt = now
	}
	def.ModifiedAt = now
	def.Source = "user"

	store := a.flowStore()
	if err := store.Save(&def); err != nil {
		return nil, err
	}
	result := flowDefToInfo(def)
	return &result, nil
}

// DeleteFlowDefinition removes a user flow definition.
func (a *App) DeleteFlowDefinition(id string) error {
	// Prevent deleting built-in flows.
	for _, def := range flow.BuiltInFlows() {
		if def.ID == id {
			return fmt.Errorf("cannot delete built-in flow %q", id)
		}
	}
	return a.flowStore().Delete(id)
}
