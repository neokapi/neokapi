package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	coreg "github.com/neokapi/neokapi/core/graph"
)

// Upsert and label-scoped delete exist for writers that rebuild a projection of
// another subsystem on every pass — the occurrence materializer keys nodes and
// edges on durable identity (a block's content key, a concept id) and writes the
// same ids again each extract, so a plain INSERT would collide. ON CONFLICT keeps
// the write idempotent and preserves created_at, so an id's first-seen time
// survives a re-materialize.

// UpsertNode inserts node, or refreshes an existing row with the same id. The
// original created_at is preserved; label, properties and updated_at follow the
// incoming node.
func (s *SQLiteGraphStore) UpsertNode(ctx context.Context, node *coreg.Node) error {
	now := time.Now()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	if node.UpdatedAt.IsZero() {
		node.UpdatedAt = now
	}
	props, err := json.Marshal(node.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	_, err = s.db.ExecContext(ctx, upsertNodeSQL,
		node.ID, node.Label, string(props),
		node.CreatedAt.Format(time.RFC3339), node.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert node: %w", err)
	}
	return nil
}

// UpsertEdge inserts edge, or refreshes an existing row with the same id.
func (s *SQLiteGraphStore) UpsertEdge(ctx context.Context, edge *coreg.Edge) error {
	now := time.Now()
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = now
	}
	if edge.UpdatedAt.IsZero() {
		edge.UpdatedAt = now
	}
	props, err := json.Marshal(edge.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	validFrom, validTo, tags := edgeValidityColumns(edge)
	_, err = s.db.ExecContext(ctx, upsertEdgeSQL,
		edge.ID, edge.Source, edge.Target, edge.Label, string(props),
		validFrom, validTo, tags,
		edge.CreatedAt.Format(time.RFC3339), edge.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert edge: %w", err)
	}
	return nil
}

// BulkUpsertNodes upserts every node in one transaction — one write permit for
// the batch, matching the single-writer discipline the merged store's write gate
// enforces per handle.
func (s *SQLiteGraphStore) BulkUpsertNodes(ctx context.Context, nodes []*coreg.Node) error {
	if len(nodes) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, upsertNodeSQL)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, node := range nodes {
		if node.CreatedAt.IsZero() {
			node.CreatedAt = now
		}
		if node.UpdatedAt.IsZero() {
			node.UpdatedAt = now
		}
		props, err := json.Marshal(node.Properties)
		if err != nil {
			return fmt.Errorf("node %s: marshal properties: %w", node.ID, err)
		}
		if _, err := stmt.ExecContext(ctx, node.ID, node.Label, string(props),
			node.CreatedAt.Format(time.RFC3339), node.UpdatedAt.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("upsert node %s: %w", node.ID, err)
		}
	}
	return tx.Commit()
}

// BulkUpsertEdges upserts every edge in one transaction.
func (s *SQLiteGraphStore) BulkUpsertEdges(ctx context.Context, edges []*coreg.Edge) error {
	if len(edges) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, upsertEdgeSQL)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for _, edge := range edges {
		if edge.CreatedAt.IsZero() {
			edge.CreatedAt = now
		}
		if edge.UpdatedAt.IsZero() {
			edge.UpdatedAt = now
		}
		props, err := json.Marshal(edge.Properties)
		if err != nil {
			return fmt.Errorf("edge %s: marshal properties: %w", edge.ID, err)
		}
		validFrom, validTo, tags := edgeValidityColumns(edge)
		if _, err := stmt.ExecContext(ctx, edge.ID, edge.Source, edge.Target, edge.Label,
			string(props), validFrom, validTo, tags,
			edge.CreatedAt.Format(time.RFC3339), edge.UpdatedAt.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("upsert edge %s: %w", edge.ID, err)
		}
	}
	return tx.Commit()
}

// DeleteEdgesByLabel removes every edge carrying one of the given labels and
// reports how many rows went. It is how a projection writer drops its own prior
// output before rebuilding: a materializer owns a fixed set of labels, so it can
// clear exactly those without touching edges another writer put in the same
// tables. Edges must go before their nodes — graph_edges.source/target reference
// graph_nodes.
func (s *SQLiteGraphStore) DeleteEdgesByLabel(ctx context.Context, labels ...string) (int64, error) {
	return s.deleteByLabel(ctx, "graph_edges", labels)
}

// DeleteNodesByLabel removes every node carrying one of the given labels. Call
// it only after the edges referencing those nodes are gone.
func (s *SQLiteGraphStore) DeleteNodesByLabel(ctx context.Context, labels ...string) (int64, error) {
	return s.deleteByLabel(ctx, "graph_nodes", labels)
}

func (s *SQLiteGraphStore) deleteByLabel(ctx context.Context, table string, labels []string) (int64, error) {
	if len(labels) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(labels))
	args := make([]any, len(labels))
	for i, l := range labels {
		placeholders[i] = "?"
		args[i] = l
	}
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM "+table+" WHERE label IN ("+strings.Join(placeholders, ",")+")", args...)
	if err != nil {
		return 0, fmt.Errorf("delete from %s by label: %w", table, err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// edgeValidityColumns renders an edge's optional temporal bounds and tags into
// the three nullable columns graph_edges stores them in.
func edgeValidityColumns(edge *coreg.Edge) (validFrom, validTo *string, tags string) {
	tags = "{}"
	if edge.Validity == nil {
		return nil, nil, tags
	}
	if edge.Validity.ValidFrom != nil {
		s := edge.Validity.ValidFrom.Format(time.RFC3339)
		validFrom = &s
	}
	if edge.Validity.ValidTo != nil {
		s := edge.Validity.ValidTo.Format(time.RFC3339)
		validTo = &s
	}
	if len(edge.Validity.Tags) > 0 {
		if b, err := json.Marshal(edge.Validity.Tags); err == nil {
			tags = string(b)
		}
	}
	return validFrom, validTo, tags
}

const upsertNodeSQL = `INSERT INTO graph_nodes (id, label, properties, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		label = excluded.label,
		properties = excluded.properties,
		updated_at = excluded.updated_at`

const upsertEdgeSQL = `INSERT INTO graph_edges (id, source, target, label, properties, valid_from, valid_to, tags, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		source = excluded.source,
		target = excluded.target,
		label = excluded.label,
		properties = excluded.properties,
		valid_from = excluded.valid_from,
		valid_to = excluded.valid_to,
		tags = excluded.tags,
		updated_at = excluded.updated_at`
