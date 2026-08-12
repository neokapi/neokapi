package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	coreg "github.com/neokapi/neokapi/core/graph"
)

// Upserts and scoped deletes: the writes a projection needs, which plain CRUD
// cannot express.
//
// A context-graph writer rebuilds a subgraph from the stores it projects, so it
// writes the same ids over and over — an insert that conflicts is the normal
// case, not an error — and it clears what it is about to write so a row that no
// longer projects does not survive. The framework's SQLite reference
// (host/storage/graph/upsert.go) carries the same four operations; this is the
// PostgreSQL dialect of them, and the two must agree, because the same writer
// shape runs over both.
//
// The delete is SCOPED, and that is the one place the two backends differ on
// purpose. The local store holds one project, so it clears a label outright;
// this store spans every workspace, project and stream, so clearing a label
// outright would wipe every tenant's subgraph. Here a delete names the scope
// properties it is entitled to remove.

// UpsertNode writes a node, replacing any node already under its id.
func (s *SQLGraphStore) UpsertNode(ctx context.Context, node *coreg.Node) error {
	return s.BulkUpsertNodes(ctx, []*coreg.Node{node})
}

// UpsertEdge writes an edge, replacing any edge already under its id.
func (s *SQLGraphStore) UpsertEdge(ctx context.Context, edge *coreg.Edge) error {
	return s.BulkUpsertEdges(ctx, []*coreg.Edge{edge})
}

// BulkUpsertNodes writes every node in one transaction. created_at is preserved
// on a row that already exists: a node re-projected from unchanged sources is
// the same node, and moving its creation instant forward would erase when the
// graph first knew about it.
func (s *SQLGraphStore) BulkUpsertNodes(ctx context.Context, nodes []*coreg.Node) error {
	if len(nodes) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, node := range nodes {
		if node.CreatedAt.IsZero() {
			node.CreatedAt = now
		}
		node.UpdatedAt = now
		batch.Queue(
			`INSERT INTO graph_nodes (id, label, properties, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (id) DO UPDATE SET
			   label = EXCLUDED.label,
			   properties = EXCLUDED.properties,
			   updated_at = EXCLUDED.updated_at`,
			node.ID, node.Label, sqlPropsJSON(node.Properties), node.CreatedAt, node.UpdatedAt)
	}

	br := tx.SendBatch(ctx, batch)
	for _, node := range nodes {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return fmt.Errorf("upsert node %s: %w", node.ID, err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close node batch: %w", err)
	}
	return tx.Commit(ctx)
}

// BulkUpsertEdges writes every edge in one transaction.
func (s *SQLGraphStore) BulkUpsertEdges(ctx context.Context, edges []*coreg.Edge) error {
	if len(edges) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, edge := range edges {
		if edge.CreatedAt.IsZero() {
			edge.CreatedAt = now
		}
		edge.UpdatedAt = now
		validity, verr := sqlValidityJSON(edge.Validity)
		if verr != nil {
			return fmt.Errorf("marshal validity for edge %s: %w", edge.ID, verr)
		}
		batch.Queue(
			`INSERT INTO graph_edges (id, source, target, label, properties, validity, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (id) DO UPDATE SET
			   source = EXCLUDED.source,
			   target = EXCLUDED.target,
			   label = EXCLUDED.label,
			   properties = EXCLUDED.properties,
			   validity = EXCLUDED.validity,
			   updated_at = EXCLUDED.updated_at`,
			edge.ID, edge.Source, edge.Target, edge.Label,
			sqlPropsJSON(edge.Properties), validity, edge.CreatedAt, edge.UpdatedAt)
	}

	br := tx.SendBatch(ctx, batch)
	for _, edge := range edges {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return fmt.Errorf("upsert edge %s: %w", edge.ID, err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close edge batch: %w", err)
	}
	return tx.Commit(ctx)
}

// DeleteEdgesByLabelInScope removes every edge with one of the given labels
// whose properties contain the scope. An empty scope is refused rather than
// treated as "everything": a writer that forgot its scope would otherwise erase
// the whole workspace's edges, and the failure would look like an empty graph.
func (s *SQLGraphStore) DeleteEdgesByLabelInScope(ctx context.Context, scope map[string]string, labels ...string) (int64, error) {
	return s.deleteByLabelInScope(ctx, "graph_edges", scope, labels)
}

// DeleteNodesByLabelInScope removes every node with one of the given labels
// whose properties contain the scope. Incident edges cascade.
func (s *SQLGraphStore) DeleteNodesByLabelInScope(ctx context.Context, scope map[string]string, labels ...string) (int64, error) {
	return s.deleteByLabelInScope(ctx, "graph_nodes", scope, labels)
}

func (s *SQLGraphStore) deleteByLabelInScope(ctx context.Context, table string, scope map[string]string, labels []string) (int64, error) {
	if len(labels) == 0 {
		return 0, nil
	}
	if len(scope) == 0 {
		return 0, fmt.Errorf("graph: delete from %s: refusing an unscoped delete over a shared graph", table)
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM `+table+` WHERE label = ANY($1) AND properties @> $2`,
		labels, sqlPropsJSON(scope))
	if err != nil {
		return 0, fmt.Errorf("delete from %s by label in scope: %w", table, err)
	}
	return tag.RowsAffected(), nil
}
