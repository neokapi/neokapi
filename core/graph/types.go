package graph

import "time"

// Direction represents edge traversal direction.
type Direction int

const (
	Outgoing Direction = iota
	Incoming
	Both
)

// Node represents a vertex in the graph.
type Node struct {
	ID         string            `json:"id"`
	Label      string            `json:"label"`
	Properties map[string]string `json:"properties"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// Edge represents a directed relationship between two nodes.
type Edge struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	Target     string            `json:"target"`
	Label      string            `json:"label"`
	Properties map[string]string `json:"properties"`
	Validity   *Validity         `json:"validity,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// Path represents an ordered sequence of nodes and edges.
type Path struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Len returns the number of edges in the path.
func (p Path) Len() int { return len(p.Edges) }

// Empty returns true if the path has no nodes.
func (p Path) Empty() bool { return len(p.Nodes) == 0 }
