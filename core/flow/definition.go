package flow

import (
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/set"
)

// Sentinel errors for flow definition validation.
var (
	ErrFlowIDRequired   = errors.New("flow definition id is required")
	ErrFlowNameRequired = errors.New("flow definition name is required")
	ErrNodeIDRequired   = errors.New("node id is required")
)

// NodeType identifies the role of a node in a flow graph.
type NodeType string

const (
	NodeReader NodeType = "reader"
	NodeWriter NodeType = "writer"
	NodeTool   NodeType = "tool"
)

// FlowDefinition is a JSON-serializable flow that can be stored and loaded.
// It captures the visual graph (nodes + edges) as well as the tool configurations
// needed to reconstruct a runnable Flow.
type FlowDefinition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Description is markdown — see markdown-in-ui.md.
	Description string     `json:"description,omitempty"`
	Nodes       []FlowNode `json:"nodes"`
	Edges       []FlowEdge `json:"edges"`
	Source      string     `json:"source"` // "built-in", "user", or "project"
	CreatedAt   string     `json:"created_at,omitempty"`
	ModifiedAt  string     `json:"modified_at,omitempty"`
	// Binding carries the flow's source/sink binding intent (AD-026). The graph
	// nodes are tools only; the I/O ends are bindings, not nodes. Nested to avoid
	// colliding with Source (the provenance field).
	Binding *FlowBinding `json:"binding,omitempty"`
}

// FlowBinding is a flow's declared source/sink binding intent (AD-026). Values
// are binding locators — "file", "store", "none", "xliff", … — matching the
// steps-format source:/sink: fields and the CLI locator vocabulary. An empty
// field means the binding is supplied at invocation, not by the flow.
type FlowBinding struct {
	Source string `json:"source,omitempty"`
	Sink   string `json:"sink,omitempty"`
}

// FlowNode represents a node in the flow graph. Tool nodes are one ordered
// list — transformers are ordinary steps (AD-006); placement safety is
// validated by ValidatePlacement, not by a structural stage.
type FlowNode struct {
	ID       string         `json:"id"`
	Type     NodeType       `json:"type"` // NodeReader, NodeWriter, or NodeTool
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
		return ErrFlowIDRequired
	}
	if d.Name == "" {
		return ErrFlowNameRequired
	}
	nodeIDs := set.New[string]()
	for _, n := range d.Nodes {
		if n.ID == "" {
			return ErrNodeIDRequired
		}
		if nodeIDs.Contains(n.ID) {
			return fmt.Errorf("duplicate node id: %s", n.ID)
		}
		nodeIDs.Add(n.ID)
		switch n.Type {
		case NodeTool, NodeReader, NodeWriter:
		default:
			return fmt.Errorf("invalid node type %q for node %s", n.Type, n.ID)
		}
	}
	for _, e := range d.Edges {
		if !nodeIDs.Contains(e.Source) {
			return fmt.Errorf("edge source %q not found in nodes", e.Source)
		}
		if !nodeIDs.Contains(e.Target) {
			return fmt.Errorf("edge target %q not found in nodes", e.Target)
		}
	}
	return nil
}

// TopologicalOrder returns node IDs in execution order following edges from
// sources to sinks. Returns an error if a cycle is detected.
func (d *FlowDefinition) TopologicalOrder() ([]string, error) {
	adj := make(map[string][]string, len(d.Nodes))
	inDeg := make(map[string]int, len(d.Nodes))
	for _, n := range d.Nodes {
		inDeg[n.ID] = 0
	}
	for _, e := range d.Edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		inDeg[e.Target]++
	}
	queue := make([]string, 0, len(d.Nodes))
	for _, n := range d.Nodes {
		if inDeg[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	order := make([]string, 0, len(d.Nodes))
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
		return nil, errors.New("cycle detected in flow graph")
	}
	return order, nil
}

// ToolNodeNames returns the names of all tool-type nodes in topological order.
func (d *FlowDefinition) ToolNodeNames() ([]string, error) {
	order, err := d.TopologicalOrder()
	if err != nil {
		return nil, err
	}
	nodeMap := make(map[string]*FlowNode, len(d.Nodes))
	for i := range d.Nodes {
		nodeMap[d.Nodes[i].ID] = &d.Nodes[i]
	}
	var names []string
	for _, id := range order {
		n := nodeMap[id]
		if n.Type == NodeTool {
			names = append(names, n.Name)
		}
	}
	return names, nil
}

// toolNodeRefs returns the flow's tool nodes in execution (topological) order
// as full nodes (name + config), so callers that need per-node config
// (data-flow contract resolution, the placement pass) have it.
func (d *FlowDefinition) toolNodeRefs() ([]FlowNode, error) {
	order, err := d.TopologicalOrder()
	if err != nil {
		return nil, err
	}
	nodeMap := make(map[string]*FlowNode, len(d.Nodes))
	for i := range d.Nodes {
		nodeMap[d.Nodes[i].ID] = &d.Nodes[i]
	}
	var refs []FlowNode
	for _, id := range order {
		n := nodeMap[id]
		if n.Type != NodeTool {
			continue
		}
		refs = append(refs, *n)
	}
	return refs, nil
}
