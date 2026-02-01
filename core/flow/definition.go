package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FlowDefinition is a JSON-serializable flow that can be stored and loaded.
// It captures the visual graph (nodes + edges) as well as the tool configurations
// needed to reconstruct a runnable Flow.
type FlowDefinition struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Nodes       []FlowNode `json:"nodes"`
	Edges       []FlowEdge `json:"edges"`
	Source      string     `json:"source"` // "built-in", "user", or "project"
	CreatedAt   string     `json:"created_at,omitempty"`
	ModifiedAt  string     `json:"modified_at,omitempty"`
}

// FlowNode represents a node in the flow graph.
type FlowNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"` // "tool", "reader", "writer"
	Name     string         `json:"name"` // tool or format name
	Label    string         `json:"label,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Position NodePosition   `json:"position"`
}

// NodePosition holds the x/y coordinates of a node in the visual graph.
type NodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// FlowEdge represents a directed edge between two nodes.
type FlowEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// Validate checks that the flow definition is well-formed.
func (d *FlowDefinition) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("flow definition id is required")
	}
	if d.Name == "" {
		return fmt.Errorf("flow definition name is required")
	}
	nodeIDs := make(map[string]bool)
	for _, n := range d.Nodes {
		if n.ID == "" {
			return fmt.Errorf("node id is required")
		}
		if nodeIDs[n.ID] {
			return fmt.Errorf("duplicate node id: %s", n.ID)
		}
		nodeIDs[n.ID] = true
		switch n.Type {
		case "tool", "reader", "writer":
		default:
			return fmt.Errorf("invalid node type %q for node %s", n.Type, n.ID)
		}
	}
	for _, e := range d.Edges {
		if !nodeIDs[e.Source] {
			return fmt.Errorf("edge source %q not found in nodes", e.Source)
		}
		if !nodeIDs[e.Target] {
			return fmt.Errorf("edge target %q not found in nodes", e.Target)
		}
	}
	return nil
}

// TopologicalOrder returns node IDs in execution order following edges from
// sources to sinks. Returns an error if a cycle is detected.
func (d *FlowDefinition) TopologicalOrder() ([]string, error) {
	adj := make(map[string][]string)
	inDeg := make(map[string]int)
	for _, n := range d.Nodes {
		inDeg[n.ID] = 0
	}
	for _, e := range d.Edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		inDeg[e.Target]++
	}
	var queue []string
	for _, n := range d.Nodes {
		if inDeg[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	var order []string
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)
		for _, next := range adj[id] {
			inDeg[next]--
			if inDeg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if len(order) != len(d.Nodes) {
		return nil, fmt.Errorf("cycle detected in flow graph")
	}
	return order, nil
}

// ToolNodeNames returns the names of all tool-type nodes in topological order.
func (d *FlowDefinition) ToolNodeNames() ([]string, error) {
	order, err := d.TopologicalOrder()
	if err != nil {
		return nil, err
	}
	nodeMap := make(map[string]*FlowNode)
	for i := range d.Nodes {
		nodeMap[d.Nodes[i].ID] = &d.Nodes[i]
	}
	var names []string
	for _, id := range order {
		n := nodeMap[id]
		if n.Type == "tool" {
			names = append(names, n.Name)
		}
	}
	return names, nil
}

// BuiltInFlows returns the default set of built-in flow definitions.
func BuiltInFlows() []FlowDefinition {
	return []FlowDefinition{
		{
			ID:          "ai-translate",
			Name:        "AI Translate",
			Description: "Translate content using AI/LLM",
			Source:      "built-in",
			Nodes: []FlowNode{
				{ID: "reader", Type: "reader", Name: "auto", Label: "Input", Position: NodePosition{X: 0, Y: 100}},
				{ID: "ai-translate", Type: "tool", Name: "ai-translate", Label: "AI Translate", Position: NodePosition{X: 250, Y: 100}},
				{ID: "writer", Type: "writer", Name: "auto", Label: "Output", Position: NodePosition{X: 500, Y: 100}},
			},
			Edges: []FlowEdge{
				{ID: "e-reader-translate", Source: "reader", Target: "ai-translate"},
				{ID: "e-translate-writer", Source: "ai-translate", Target: "writer"},
			},
		},
		{
			ID:          "ai-translate-qa",
			Name:        "AI Translate + QA",
			Description: "Translate content using AI/LLM then run quality check",
			Source:      "built-in",
			Nodes: []FlowNode{
				{ID: "reader", Type: "reader", Name: "auto", Label: "Input", Position: NodePosition{X: 0, Y: 100}},
				{ID: "ai-translate", Type: "tool", Name: "ai-translate", Label: "AI Translate", Position: NodePosition{X: 250, Y: 100}},
				{ID: "ai-qa", Type: "tool", Name: "ai-qa", Label: "QA Check", Position: NodePosition{X: 500, Y: 100}},
				{ID: "writer", Type: "writer", Name: "auto", Label: "Output", Position: NodePosition{X: 750, Y: 100}},
			},
			Edges: []FlowEdge{
				{ID: "e-reader-translate", Source: "reader", Target: "ai-translate"},
				{ID: "e-translate-qa", Source: "ai-translate", Target: "ai-qa"},
				{ID: "e-qa-writer", Source: "ai-qa", Target: "writer"},
			},
		},
		{
			ID:          "pseudo-translate",
			Name:        "Pseudo Translate",
			Description: "Generate pseudo-translations for testing",
			Source:      "built-in",
			Nodes: []FlowNode{
				{ID: "reader", Type: "reader", Name: "auto", Label: "Input", Position: NodePosition{X: 0, Y: 100}},
				{ID: "pseudo-translate", Type: "tool", Name: "pseudo-translate", Label: "Pseudo Translate", Position: NodePosition{X: 250, Y: 100}},
				{ID: "writer", Type: "writer", Name: "auto", Label: "Output", Position: NodePosition{X: 500, Y: 100}},
			},
			Edges: []FlowEdge{
				{ID: "e-reader-pseudo", Source: "reader", Target: "pseudo-translate"},
				{ID: "e-pseudo-writer", Source: "pseudo-translate", Target: "writer"},
			},
		},
	}
}

// FlowStore manages persistent storage of user flow definitions.
type FlowStore struct {
	dir string
}

// NewFlowStore creates a FlowStore that reads/writes JSON files from the given directory.
func NewFlowStore(dir string) *FlowStore {
	return &FlowStore{dir: dir}
}

// List returns all user flow definitions in the store.
func (s *FlowStore) List() ([]FlowDefinition, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read flow dir: %w", err)
	}
	var defs []FlowDefinition
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var def FlowDefinition
		if err := json.Unmarshal(data, &def); err != nil {
			continue
		}
		def.Source = "user"
		defs = append(defs, def)
	}
	return defs, nil
}

// Get returns a specific flow definition by ID.
func (s *FlowStore) Get(id string) (*FlowDefinition, error) {
	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("flow %q not found", id)
		}
		return nil, err
	}
	var def FlowDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse flow %q: %w", id, err)
	}
	def.Source = "user"
	return &def, nil
}

// Save writes a flow definition to the store.
func (s *FlowStore) Save(def *FlowDefinition) error {
	if err := def.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create flow dir: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if def.CreatedAt == "" {
		def.CreatedAt = now
	}
	def.ModifiedAt = now
	def.Source = "user"

	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal flow: %w", err)
	}
	path := filepath.Join(s.dir, def.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// Delete removes a flow definition from the store.
func (s *FlowStore) Delete(id string) error {
	path := filepath.Join(s.dir, id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("flow %q not found", id)
		}
		return err
	}
	return nil
}
