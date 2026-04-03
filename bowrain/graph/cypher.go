package graph

import (
	"context"

	coreg "github.com/neokapi/neokapi/core/graph"
)

// CypherStore extends GraphStore with native Cypher query support.
// Only the AGE backend implements this interface; callers should
// type-assert when they need Cypher access.
type CypherStore interface {
	coreg.GraphStore
	CypherQuery(ctx context.Context, query string, params map[string]any) ([]*coreg.Node, error)
	CypherExec(ctx context.Context, query string, params map[string]any) error
}
