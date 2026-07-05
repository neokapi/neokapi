package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/storage"
)

// SQLiteGraphStore implements graph.GraphStore using SQLite adjacency tables.
type SQLiteGraphStore struct {
	db *storage.DB
}

var migrations = []storage.Migration{
	{
		Version:     1,
		Description: "graph store schema with adjacency tables",
		SQL: `
		CREATE TABLE IF NOT EXISTS graph_nodes (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			properties TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS graph_edges (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL REFERENCES graph_nodes(id),
			target TEXT NOT NULL REFERENCES graph_nodes(id),
			label TEXT NOT NULL,
			properties TEXT NOT NULL DEFAULT '{}',
			valid_from TEXT,
			valid_to TEXT,
			tags TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_graph_edges_source ON graph_edges(source);
		CREATE INDEX IF NOT EXISTS idx_graph_edges_target ON graph_edges(target);
		CREATE INDEX IF NOT EXISTS idx_graph_edges_label ON graph_edges(label);
		CREATE INDEX IF NOT EXISTS idx_graph_nodes_label ON graph_nodes(label);
		`,
	},
}

// NewSQLiteGraphStore creates a new SQLite-backed graph store.
func NewSQLiteGraphStore(db *storage.DB) (*SQLiteGraphStore, error) {
	if err := storage.Migrate(db, "graph", migrations); err != nil {
		return nil, fmt.Errorf("graph migration: %w", err)
	}
	return &SQLiteGraphStore{db: db}, nil
}

func (s *SQLiteGraphStore) CreateNode(ctx context.Context, node *coreg.Node) error {
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
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO graph_nodes (id, label, properties, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		node.ID, node.Label, string(props),
		node.CreatedAt.Format(time.RFC3339), node.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert node: %w", err)
	}
	return nil
}

func (s *SQLiteGraphStore) GetNode(ctx context.Context, id string) (*coreg.Node, error) {
	var n coreg.Node
	var propsJSON, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, label, properties, created_at, updated_at FROM graph_nodes WHERE id = ?`, id).
		Scan(&n.ID, &n.Label, &propsJSON, &createdStr, &updatedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, coreg.ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get node: %w", err)
	}
	_ = json.Unmarshal([]byte(propsJSON), &n.Properties)
	n.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &n, nil
}

func (s *SQLiteGraphStore) UpdateNode(ctx context.Context, node *coreg.Node) error {
	node.UpdatedAt = time.Now()
	props, err := json.Marshal(node.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE graph_nodes SET label = ?, properties = ?, updated_at = ? WHERE id = ?`,
		node.Label, string(props), node.UpdatedAt.Format(time.RFC3339), node.ID)
	if err != nil {
		return fmt.Errorf("update node: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return coreg.ErrNodeNotFound
	}
	return nil
}

func (s *SQLiteGraphStore) DeleteNode(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM graph_nodes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return coreg.ErrNodeNotFound
	}
	return nil
}

func (s *SQLiteGraphStore) FindNodes(ctx context.Context, label string, properties map[string]string) ([]*coreg.Node, error) {
	where := "label = ?"
	args := []any{label}
	var whereSb137 strings.Builder
	for k, v := range properties {
		whereSb137.WriteString(" AND json_extract(properties, ?) = ?")
		args = append(args, "$."+k, v)
	}
	where += whereSb137.String()
	return s.queryNodes(ctx, where, args)
}

func (s *SQLiteGraphStore) FindNodesScoped(ctx context.Context, label string, properties map[string]string, scope coreg.Scope) ([]*coreg.Node, error) {
	// FindNodesScoped finds nodes that have at least one active edge matching the scope.
	// First find matching nodes by label/properties, then filter by edge validity.
	nodes, err := s.FindNodes(ctx, label, properties)
	if err != nil {
		return nil, err
	}
	// Filter nodes that participate in at least one edge matching scope.
	var result []*coreg.Node
	for _, n := range nodes {
		edges, err := s.EdgesOf(ctx, n.ID, coreg.Both)
		if err != nil {
			continue
		}
		hasActive := len(edges) == 0 // nodes with no edges always match
		for _, e := range edges {
			if e.Validity.Matches(scope) {
				hasActive = true
				break
			}
		}
		if hasActive {
			result = append(result, n)
		}
	}
	return result, nil
}

func (s *SQLiteGraphStore) CreateEdge(ctx context.Context, edge *coreg.Edge) error {
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
	var validFrom, validTo *string
	tags := "{}"
	if edge.Validity != nil {
		if edge.Validity.ValidFrom != nil {
			s := edge.Validity.ValidFrom.Format(time.RFC3339)
			validFrom = &s
		}
		if edge.Validity.ValidTo != nil {
			s := edge.Validity.ValidTo.Format(time.RFC3339)
			validTo = &s
		}
		if len(edge.Validity.Tags) > 0 {
			b, _ := json.Marshal(edge.Validity.Tags)
			tags = string(b)
		}
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO graph_edges (id, source, target, label, properties, valid_from, valid_to, tags, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		edge.ID, edge.Source, edge.Target, edge.Label, string(props),
		validFrom, validTo, tags,
		edge.CreatedAt.Format(time.RFC3339), edge.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert edge: %w", err)
	}
	return nil
}

func (s *SQLiteGraphStore) GetEdge(ctx context.Context, id string) (*coreg.Edge, error) {
	return s.scanEdge(ctx, `SELECT id, source, target, label, properties, valid_from, valid_to, tags, created_at, updated_at FROM graph_edges WHERE id = ?`, id)
}

func (s *SQLiteGraphStore) UpdateEdge(ctx context.Context, edge *coreg.Edge) error {
	edge.UpdatedAt = time.Now()
	props, err := json.Marshal(edge.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	var validFrom, validTo *string
	tags := "{}"
	if edge.Validity != nil {
		if edge.Validity.ValidFrom != nil {
			s := edge.Validity.ValidFrom.Format(time.RFC3339)
			validFrom = &s
		}
		if edge.Validity.ValidTo != nil {
			s := edge.Validity.ValidTo.Format(time.RFC3339)
			validTo = &s
		}
		if len(edge.Validity.Tags) > 0 {
			b, _ := json.Marshal(edge.Validity.Tags)
			tags = string(b)
		}
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE graph_edges SET source = ?, target = ?, label = ?, properties = ?, valid_from = ?, valid_to = ?, tags = ?, updated_at = ? WHERE id = ?`,
		edge.Source, edge.Target, edge.Label, string(props),
		validFrom, validTo, tags,
		edge.UpdatedAt.Format(time.RFC3339), edge.ID)
	if err != nil {
		return fmt.Errorf("update edge: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return coreg.ErrEdgeNotFound
	}
	return nil
}

func (s *SQLiteGraphStore) DeleteEdge(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM graph_edges WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete edge: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return coreg.ErrEdgeNotFound
	}
	return nil
}

func (s *SQLiteGraphStore) FindEdges(ctx context.Context, label string, properties map[string]string) ([]*coreg.Edge, error) {
	where := "label = ?"
	args := []any{label}
	var whereSb268 strings.Builder
	for k, v := range properties {
		whereSb268.WriteString(" AND json_extract(properties, ?) = ?")
		args = append(args, "$."+k, v)
	}
	where += whereSb268.String()
	return s.queryEdges(ctx, where, args)
}

func (s *SQLiteGraphStore) Neighbors(ctx context.Context, nodeID string, direction coreg.Direction, labels ...string) ([]*coreg.Node, error) {
	where, args := s.neighborsWhere(nodeID, direction, labels)
	return s.queryNodes(ctx,
		fmt.Sprintf("id IN (SELECT CASE WHEN source = ? THEN target ELSE source END FROM graph_edges WHERE %s)", where),
		append([]any{nodeID}, args...))
}

func (s *SQLiteGraphStore) NeighborsScoped(ctx context.Context, nodeID string, direction coreg.Direction, scope coreg.Scope, labels ...string) ([]*coreg.Node, error) {
	where, args := s.neighborsWhere(nodeID, direction, labels)
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, source, target, label, properties, valid_from, valid_to, tags, created_at, updated_at FROM graph_edges WHERE "+where,
		args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	nodeIDs := make(map[string]bool)
	for rows.Next() {
		e, err := scanEdgeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan edge row: %w", err)
		}
		if e.Validity.Matches(scope) {
			if e.Source == nodeID {
				nodeIDs[e.Target] = true
			} else {
				nodeIDs[e.Source] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate edges: %w", err)
	}

	var result []*coreg.Node
	for nid := range nodeIDs {
		n, err := s.GetNode(ctx, nid)
		if err == nil {
			result = append(result, n)
		}
	}
	return result, nil
}

func (s *SQLiteGraphStore) EdgesOf(ctx context.Context, nodeID string, direction coreg.Direction, labels ...string) ([]*coreg.Edge, error) {
	where, args := s.neighborsWhere(nodeID, direction, labels)
	return s.queryEdges(ctx, where, args)
}

func (s *SQLiteGraphStore) ShortestPath(ctx context.Context, fromID, toID string, maxDepth int) (*coreg.Path, error) {
	if maxDepth <= 0 {
		maxDepth = 10
	}

	// BFS using recursive CTE
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE bfs(node, depth, path_nodes, path_edges) AS (
			SELECT ?, 0, ?, ''
			UNION ALL
			SELECT
				CASE WHEN e.source = bfs.node THEN e.target ELSE e.source END,
				bfs.depth + 1,
				bfs.path_nodes || ',' || CASE WHEN e.source = bfs.node THEN e.target ELSE e.source END,
				CASE WHEN bfs.path_edges = '' THEN e.id ELSE bfs.path_edges || ',' || e.id END
			FROM bfs
			JOIN graph_edges e ON (e.source = bfs.node OR e.target = bfs.node)
			WHERE bfs.depth < ?
			  AND instr(bfs.path_nodes, CASE WHEN e.source = bfs.node THEN e.target ELSE e.source END) = 0
		)
		SELECT path_nodes, path_edges FROM bfs WHERE node = ? LIMIT 1
	`, fromID, fromID, maxDepth, toID)
	if err != nil {
		return nil, fmt.Errorf("shortest path query: %w", err)
	}
	defer rows.Close()

	var pathNodesStr, pathEdgesStr string
	found := false
	if rows.Next() {
		if err := rows.Scan(&pathNodesStr, &pathEdgesStr); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan path: %w", err)
		}
		found = true
	}
	rows.Close() // release connection before fetching nodes/edges

	if !found {
		return nil, nil // no path found
	}

	path := &coreg.Path{}
	for nid := range strings.SplitSeq(pathNodesStr, ",") {
		if nid == "" {
			continue
		}
		n, err := s.GetNode(ctx, nid)
		if err != nil {
			return nil, fmt.Errorf("get path node %s: %w", nid, err)
		}
		path.Nodes = append(path.Nodes, *n)
	}
	if pathEdgesStr != "" {
		for eid := range strings.SplitSeq(pathEdgesStr, ",") {
			if eid == "" {
				continue
			}
			e, err := s.GetEdge(ctx, eid)
			if err != nil {
				return nil, fmt.Errorf("get path edge %s: %w", eid, err)
			}
			path.Edges = append(path.Edges, *e)
		}
	}
	return path, nil
}

func (s *SQLiteGraphStore) BulkCreateNodes(ctx context.Context, nodes []*coreg.Node) error {
	tx, err := s.db.Begin() //nolint:noctx // batch graph operation
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO graph_nodes (id, label, properties, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`)
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
		props, _ := json.Marshal(node.Properties)
		if _, err := stmt.ExecContext(ctx, node.ID, node.Label, string(props),
			node.CreatedAt.Format(time.RFC3339), node.UpdatedAt.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("insert node %s: %w", node.ID, err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteGraphStore) BulkCreateEdges(ctx context.Context, edges []*coreg.Edge) error {
	tx, err := s.db.Begin() //nolint:noctx // batch graph operation
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO graph_edges (id, source, target, label, properties, valid_from, valid_to, tags, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
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
		props, _ := json.Marshal(edge.Properties)
		var validFrom, validTo *string
		tags := "{}"
		if edge.Validity != nil {
			if edge.Validity.ValidFrom != nil {
				vf := edge.Validity.ValidFrom.Format(time.RFC3339)
				validFrom = &vf
			}
			if edge.Validity.ValidTo != nil {
				vt := edge.Validity.ValidTo.Format(time.RFC3339)
				validTo = &vt
			}
			if len(edge.Validity.Tags) > 0 {
				b, _ := json.Marshal(edge.Validity.Tags)
				tags = string(b)
			}
		}
		if _, err := stmt.ExecContext(ctx, edge.ID, edge.Source, edge.Target, edge.Label,
			string(props), validFrom, validTo, tags,
			edge.CreatedAt.Format(time.RFC3339), edge.UpdatedAt.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("insert edge %s: %w", edge.ID, err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteGraphStore) CypherQuery(_ context.Context, _ string, _ map[string]any) ([]*coreg.Node, error) {
	return nil, coreg.ErrCypherNotSupported
}

func (s *SQLiteGraphStore) CypherExec(_ context.Context, _ string, _ map[string]any) error {
	return coreg.ErrCypherNotSupported
}

func (s *SQLiteGraphStore) Close() error {
	return s.db.Close()
}

// --- internal helpers ---

func (s *SQLiteGraphStore) neighborsWhere(nodeID string, direction coreg.Direction, labels []string) (string, []any) {
	var clauses []string
	var args []any

	switch direction {
	case coreg.Outgoing:
		clauses = append(clauses, "source = ?")
		args = append(args, nodeID)
	case coreg.Incoming:
		clauses = append(clauses, "target = ?")
		args = append(args, nodeID)
	case coreg.Both:
		clauses = append(clauses, "(source = ? OR target = ?)")
		args = append(args, nodeID, nodeID)
	}

	if len(labels) > 0 {
		placeholders := make([]string, len(labels))
		for i, l := range labels {
			placeholders[i] = "?"
			args = append(args, l)
		}
		clauses = append(clauses, "label IN ("+strings.Join(placeholders, ",")+")")
	}

	return strings.Join(clauses, " AND "), args
}

func (s *SQLiteGraphStore) queryNodes(ctx context.Context, where string, args []any) ([]*coreg.Node, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, label, properties, created_at, updated_at FROM graph_nodes WHERE "+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*coreg.Node
	for rows.Next() {
		var n coreg.Node
		var propsJSON, createdStr, updatedStr string
		if err := rows.Scan(&n.ID, &n.Label, &propsJSON, &createdStr, &updatedStr); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(propsJSON), &n.Properties)
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		nodes = append(nodes, &n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nodes: %w", err)
	}
	return nodes, nil
}

func (s *SQLiteGraphStore) queryEdges(ctx context.Context, where string, args []any) ([]*coreg.Edge, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, source, target, label, properties, valid_from, valid_to, tags, created_at, updated_at FROM graph_edges WHERE "+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	var edges []*coreg.Edge
	for rows.Next() {
		e, err := scanEdgeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan edge row: %w", err)
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate edges: %w", err)
	}
	return edges, nil
}

func (s *SQLiteGraphStore) scanEdge(ctx context.Context, query string, args ...any) (*coreg.Edge, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	var e coreg.Edge
	var propsJSON, tags, createdStr, updatedStr string
	var validFrom, validTo *string
	err := row.Scan(&e.ID, &e.Source, &e.Target, &e.Label, &propsJSON, &validFrom, &validTo, &tags, &createdStr, &updatedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, coreg.ErrEdgeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan edge: %w", err)
	}
	_ = json.Unmarshal([]byte(propsJSON), &e.Properties)
	e.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	e.Validity = parseValidity(validFrom, validTo, tags)
	return &e, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEdgeRow(row scanner) (*coreg.Edge, error) {
	var e coreg.Edge
	var propsJSON, tags, createdStr, updatedStr string
	var validFrom, validTo *string
	if err := row.Scan(&e.ID, &e.Source, &e.Target, &e.Label, &propsJSON, &validFrom, &validTo, &tags, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(propsJSON), &e.Properties)
	e.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	e.Validity = parseValidity(validFrom, validTo, tags)
	return &e, nil
}

func parseValidity(validFrom, validTo *string, tagsJSON string) *coreg.Validity {
	var vf, vt *time.Time
	if validFrom != nil {
		t, err := time.Parse(time.RFC3339, *validFrom)
		if err == nil {
			vf = &t
		}
	}
	if validTo != nil {
		t, err := time.Parse(time.RFC3339, *validTo)
		if err == nil {
			vt = &t
		}
	}
	var tags map[string]string
	if tagsJSON != "" && tagsJSON != "{}" {
		_ = json.Unmarshal([]byte(tagsJSON), &tags)
	}
	if vf == nil && vt == nil && len(tags) == 0 {
		return nil
	}
	return &coreg.Validity{ValidFrom: vf, ValidTo: vt, Tags: tags}
}
