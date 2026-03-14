package graph

import (
	"context"
	"errors"
)

// GraphStore defines the interface for graph storage backends.
// Implementations include SQLite (framework/cli) and Apache AGE (platform).
type GraphStore interface {
	// Node CRUD
	CreateNode(ctx context.Context, node *Node) error
	GetNode(ctx context.Context, id string) (*Node, error)
	UpdateNode(ctx context.Context, node *Node) error
	DeleteNode(ctx context.Context, id string) error

	// Node queries
	FindNodes(ctx context.Context, label string, properties map[string]string) ([]*Node, error)
	FindNodesScoped(ctx context.Context, label string, properties map[string]string, scope Scope) ([]*Node, error)

	// Edge CRUD
	CreateEdge(ctx context.Context, edge *Edge) error
	GetEdge(ctx context.Context, id string) (*Edge, error)
	UpdateEdge(ctx context.Context, edge *Edge) error
	DeleteEdge(ctx context.Context, id string) error

	// Edge queries
	FindEdges(ctx context.Context, label string, properties map[string]string) ([]*Edge, error)

	// Traversal
	Neighbors(ctx context.Context, nodeID string, direction Direction, labels ...string) ([]*Node, error)
	NeighborsScoped(ctx context.Context, nodeID string, direction Direction, scope Scope, labels ...string) ([]*Node, error)
	EdgesOf(ctx context.Context, nodeID string, direction Direction, labels ...string) ([]*Edge, error)

	// Path queries
	ShortestPath(ctx context.Context, fromID, toID string, maxDepth int) (*Path, error)

	// Bulk operations
	BulkCreateNodes(ctx context.Context, nodes []*Node) error
	BulkCreateEdges(ctx context.Context, edges []*Edge) error

	// Cypher escape hatch (AGE backend only; SQLite returns ErrCypherNotSupported)
	CypherQuery(ctx context.Context, query string, params map[string]any) ([]*Node, error)
	CypherExec(ctx context.Context, query string, params map[string]any) error

	// Lifecycle
	Close() error
}

// ErrCypherNotSupported is returned by SQLite backends when Cypher operations are attempted.
var ErrCypherNotSupported = errors.New("cypher queries require a graph database backend with native Cypher support")

// ErrNodeNotFound is returned when a node lookup fails.
var ErrNodeNotFound = errors.New("node not found")

// ErrEdgeNotFound is returned when an edge lookup fails.
var ErrEdgeNotFound = errors.New("edge not found")
