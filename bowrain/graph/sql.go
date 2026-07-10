package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreg "github.com/neokapi/neokapi/core/graph"
)

// SQLGraphStore implements coreg.GraphStore on standard PostgreSQL using plain
// relational adjacency tables (graph_nodes + graph_edges) — no Apache AGE, no
// Cypher. It is a dialect port of the framework's SQLite reference
// (host/storage/graph/sqlite.go): the same node/edge CRUD, jsonb property
// containment, temporal Scope/Validity filtering (evaluated in Go via
// Validity.Matches for byte-for-byte parity with the reference), directional
// neighbours, and a bounded, cycle-safe recursive-CTE shortest path.
//
// SQL becomes the default backend so both local dev and managed RDS/Aurora —
// which do not ship the AGE extension — run on stock PostgreSQL. AGE remains
// available as an opt-in backend (AGEGraphStore) and is the upgrade path if the
// workload ever needs native graph traversal.
//
// The store is backed by the *same* *pgxpool.Pool the rest of the relational
// stores use — each call acquires its own connection from that pool, so graph
// and relational writes are independent autocommits, not one shared
// transaction. The pool is owned by the caller (storage.PgDB); Close is
// therefore a no-op.
type SQLGraphStore struct {
	pool *pgxpool.Pool
}

var _ coreg.GraphStore = (*SQLGraphStore)(nil)

// NewSQLGraphStore creates a plain-SQL PostgreSQL graph store over the given
// pool. Call EnsureGraph once (at wiring time) to create the schema.
func NewSQLGraphStore(pool *pgxpool.Pool) *SQLGraphStore {
	return &SQLGraphStore{pool: pool}
}

// graphSchemaLockKey is an arbitrary, fixed advisory-lock key shared by every
// bowrain-server process. EnsureGraph holds it as a transaction-scoped advisory
// lock so concurrent cold-starts (multiple replicas booting against one fresh
// RDS database) serialise their DDL instead of racing on the shared catalog,
// where `CREATE TABLE/INDEX IF NOT EXISTS` can otherwise raise SQLSTATE 23505 or
// "tuple concurrently updated" on the loser.
const graphSchemaLockKey int64 = 0x6272676170687364

// EnsureGraph creates the graph schema if it does not already exist. It is
// idempotent (CREATE TABLE IF NOT EXISTS) and self-contained — deliberately not
// registered in bowrain/store/migrations.go so it cannot collide with other
// branches' migration versions. The DDL runs under a transaction-scoped
// advisory lock so multiple replicas cold-starting against one fresh database
// serialise rather than race on the shared catalog.
//
// graph_edges' source/target reference graph_nodes with ON DELETE CASCADE, so a
// DeleteNode removes the node's incident edges in one step — matching the AGE
// backend's DETACH DELETE and restoring the referential integrity the SQLite
// reference enforces via its foreign keys. Without it, a deleted node would
// leave orphan edges that surface as ErrNodeNotFound during ShortestPath
// reconstruction.
func (s *SQLGraphStore) EnsureGraph(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS graph_nodes (
	id         text PRIMARY KEY,
	label      text NOT NULL,
	properties jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS graph_edges (
	id         text PRIMARY KEY,
	source     text NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
	target     text NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
	label      text NOT NULL,
	properties jsonb NOT NULL DEFAULT '{}'::jsonb,
	validity   jsonb,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_graph_nodes_label ON graph_nodes(label);
CREATE INDEX IF NOT EXISTS idx_graph_nodes_props ON graph_nodes USING gin (properties);
CREATE INDEX IF NOT EXISTS idx_graph_edges_source ON graph_edges(source);
CREATE INDEX IF NOT EXISTS idx_graph_edges_target ON graph_edges(target);
CREATE INDEX IF NOT EXISTS idx_graph_edges_label ON graph_edges(label);
CREATE INDEX IF NOT EXISTS idx_graph_edges_props ON graph_edges USING gin (properties);
`
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ensure graph schema: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialise concurrent cold-starts; the lock is released on commit/rollback.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, graphSchemaLockKey); err != nil {
		return fmt.Errorf("ensure graph schema: acquire lock: %w", err)
	}
	if _, err := tx.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("ensure graph schema: %w", err)
	}
	return tx.Commit(ctx)
}

// --------------------------------------------------------------------------
// Node CRUD
// --------------------------------------------------------------------------

func (s *SQLGraphStore) CreateNode(ctx context.Context, node *coreg.Node) error {
	now := time.Now().UTC()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	if node.UpdatedAt.IsZero() {
		node.UpdatedAt = now
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO graph_nodes (id, label, properties, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		node.ID, node.Label, sqlPropsJSON(node.Properties), node.CreatedAt, node.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert node: %w", err)
	}
	return nil
}

func (s *SQLGraphStore) GetNode(ctx context.Context, id string) (*coreg.Node, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, label, properties, created_at, updated_at FROM graph_nodes WHERE id = $1`, id)
	n, err := scanSQLNode(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, coreg.ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get node: %w", err)
	}
	return n, nil
}

func (s *SQLGraphStore) UpdateNode(ctx context.Context, node *coreg.Node) error {
	node.UpdatedAt = time.Now().UTC()
	tag, err := s.pool.Exec(ctx,
		`UPDATE graph_nodes SET label = $1, properties = $2, updated_at = $3 WHERE id = $4`,
		node.Label, sqlPropsJSON(node.Properties), node.UpdatedAt, node.ID)
	if err != nil {
		return fmt.Errorf("update node: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return coreg.ErrNodeNotFound
	}
	return nil
}

func (s *SQLGraphStore) DeleteNode(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM graph_nodes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return coreg.ErrNodeNotFound
	}
	return nil
}

// --------------------------------------------------------------------------
// Node queries
// --------------------------------------------------------------------------

func (s *SQLGraphStore) FindNodes(ctx context.Context, label string, properties map[string]string) ([]*coreg.Node, error) {
	where := "label = $1"
	args := []any{label}
	if len(properties) > 0 {
		args = append(args, sqlPropsJSON(properties))
		where += fmt.Sprintf(" AND properties @> $%d", len(args))
	}
	return s.queryNodes(ctx, where, args)
}

func (s *SQLGraphStore) FindNodesScoped(ctx context.Context, label string, properties map[string]string, scope coreg.Scope) ([]*coreg.Node, error) {
	// Mirrors the SQLite reference: find candidate nodes by label/properties,
	// then keep those that participate in at least one edge active at the scope.
	// A node with no edges always matches (nothing constrains it).
	nodes, err := s.FindNodes(ctx, label, properties)
	if err != nil {
		return nil, err
	}
	var result []*coreg.Node
	for _, n := range nodes {
		edges, err := s.EdgesOf(ctx, n.ID, coreg.Both)
		if err != nil {
			continue
		}
		hasActive := len(edges) == 0
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

// --------------------------------------------------------------------------
// Edge CRUD
// --------------------------------------------------------------------------

func (s *SQLGraphStore) CreateEdge(ctx context.Context, edge *coreg.Edge) error {
	now := time.Now().UTC()
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = now
	}
	if edge.UpdatedAt.IsZero() {
		edge.UpdatedAt = now
	}
	validity, err := sqlValidityJSON(edge.Validity)
	if err != nil {
		return fmt.Errorf("marshal validity: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO graph_edges (id, source, target, label, properties, validity, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		edge.ID, edge.Source, edge.Target, edge.Label,
		sqlPropsJSON(edge.Properties), validity, edge.CreatedAt, edge.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert edge: %w", err)
	}
	return nil
}

func (s *SQLGraphStore) GetEdge(ctx context.Context, id string) (*coreg.Edge, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, source, target, label, properties, validity, created_at, updated_at
		 FROM graph_edges WHERE id = $1`, id)
	e, err := scanSQLEdge(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, coreg.ErrEdgeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get edge: %w", err)
	}
	return e, nil
}

func (s *SQLGraphStore) UpdateEdge(ctx context.Context, edge *coreg.Edge) error {
	edge.UpdatedAt = time.Now().UTC()
	validity, err := sqlValidityJSON(edge.Validity)
	if err != nil {
		return fmt.Errorf("marshal validity: %w", err)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE graph_edges
		 SET source = $1, target = $2, label = $3, properties = $4, validity = $5, updated_at = $6
		 WHERE id = $7`,
		edge.Source, edge.Target, edge.Label, sqlPropsJSON(edge.Properties),
		validity, edge.UpdatedAt, edge.ID)
	if err != nil {
		return fmt.Errorf("update edge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return coreg.ErrEdgeNotFound
	}
	return nil
}

func (s *SQLGraphStore) DeleteEdge(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM graph_edges WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete edge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return coreg.ErrEdgeNotFound
	}
	return nil
}

// --------------------------------------------------------------------------
// Edge queries
// --------------------------------------------------------------------------

func (s *SQLGraphStore) FindEdges(ctx context.Context, label string, properties map[string]string) ([]*coreg.Edge, error) {
	where := "label = $1"
	args := []any{label}
	if len(properties) > 0 {
		args = append(args, sqlPropsJSON(properties))
		where += fmt.Sprintf(" AND properties @> $%d", len(args))
	}
	return s.queryEdges(ctx, where, args)
}

// --------------------------------------------------------------------------
// Traversal
// --------------------------------------------------------------------------

func (s *SQLGraphStore) Neighbors(ctx context.Context, nodeID string, direction coreg.Direction, labels ...string) ([]*coreg.Node, error) {
	edges, err := s.EdgesOf(ctx, nodeID, direction, labels...)
	if err != nil {
		return nil, err
	}
	return s.nodesFromEdges(ctx, nodeID, edges, nil)
}

func (s *SQLGraphStore) NeighborsScoped(ctx context.Context, nodeID string, direction coreg.Direction, scope coreg.Scope, labels ...string) ([]*coreg.Node, error) {
	edges, err := s.EdgesOf(ctx, nodeID, direction, labels...)
	if err != nil {
		return nil, err
	}
	return s.nodesFromEdges(ctx, nodeID, edges, &scope)
}

func (s *SQLGraphStore) EdgesOf(ctx context.Context, nodeID string, direction coreg.Direction, labels ...string) ([]*coreg.Edge, error) {
	where, args := s.neighborWhere(nodeID, direction, labels)
	return s.queryEdges(ctx, where, args)
}

// --------------------------------------------------------------------------
// Path queries
// --------------------------------------------------------------------------

// ShortestPath finds a shortest path between two nodes using a bounded,
// cycle-safe breadth-first recursive CTE. It carries the visited-node path as a
// text[] and refuses to revisit a node already on the path, so it always
// terminates even on cyclic graphs. Returns nil (not an empty Path) when no
// path exists within maxDepth — mirroring the SQLite reference.
func (s *SQLGraphStore) ShortestPath(ctx context.Context, fromID, toID string, maxDepth int) (*coreg.Path, error) {
	if maxDepth <= 0 {
		maxDepth = 10
	}

	const q = `
WITH RECURSIVE bfs(node, depth, path_nodes, path_edges) AS (
	SELECT $1::text, 0, ARRAY[$1::text], ARRAY[]::text[]
	UNION ALL
	SELECT
		CASE WHEN e.source = b.node THEN e.target ELSE e.source END,
		b.depth + 1,
		b.path_nodes || (CASE WHEN e.source = b.node THEN e.target ELSE e.source END),
		b.path_edges || e.id
	FROM bfs b
	JOIN graph_edges e ON (e.source = b.node OR e.target = b.node)
	WHERE b.depth < $2
	  AND NOT ((CASE WHEN e.source = b.node THEN e.target ELSE e.source END) = ANY(b.path_nodes))
)
SELECT path_nodes, path_edges
FROM bfs
WHERE node = $3
ORDER BY depth
LIMIT 1
`
	var pathNodeIDs, pathEdgeIDs []string
	err := s.pool.QueryRow(ctx, q, fromID, maxDepth, toID).Scan(&pathNodeIDs, &pathEdgeIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // unreachable within maxDepth
	}
	if err != nil {
		return nil, fmt.Errorf("shortest path query: %w", err)
	}

	path := &coreg.Path{}
	for _, nid := range pathNodeIDs {
		if nid == "" {
			continue
		}
		n, err := s.GetNode(ctx, nid)
		if err != nil {
			return nil, fmt.Errorf("get path node %s: %w", nid, err)
		}
		path.Nodes = append(path.Nodes, *n)
	}
	for _, eid := range pathEdgeIDs {
		if eid == "" {
			continue
		}
		e, err := s.GetEdge(ctx, eid)
		if err != nil {
			return nil, fmt.Errorf("get path edge %s: %w", eid, err)
		}
		path.Edges = append(path.Edges, *e)
	}
	return path, nil
}

// --------------------------------------------------------------------------
// Bulk operations
// --------------------------------------------------------------------------

func (s *SQLGraphStore) BulkCreateNodes(ctx context.Context, nodes []*coreg.Node) error {
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
		if node.UpdatedAt.IsZero() {
			node.UpdatedAt = now
		}
		batch.Queue(
			`INSERT INTO graph_nodes (id, label, properties, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			node.ID, node.Label, sqlPropsJSON(node.Properties), node.CreatedAt, node.UpdatedAt)
	}

	br := tx.SendBatch(ctx, batch)
	for _, node := range nodes {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return fmt.Errorf("insert node %s: %w", node.ID, err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close node batch: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *SQLGraphStore) BulkCreateEdges(ctx context.Context, edges []*coreg.Edge) error {
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
		if edge.UpdatedAt.IsZero() {
			edge.UpdatedAt = now
		}
		validity, err := sqlValidityJSON(edge.Validity)
		if err != nil {
			return fmt.Errorf("marshal validity for edge %s: %w", edge.ID, err)
		}
		batch.Queue(
			`INSERT INTO graph_edges (id, source, target, label, properties, validity, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			edge.ID, edge.Source, edge.Target, edge.Label,
			sqlPropsJSON(edge.Properties), validity, edge.CreatedAt, edge.UpdatedAt)
	}

	br := tx.SendBatch(ctx, batch)
	for _, edge := range edges {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return fmt.Errorf("insert edge %s: %w", edge.ID, err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close edge batch: %w", err)
	}
	return tx.Commit(ctx)
}

// --------------------------------------------------------------------------
// Cypher escape hatch — unsupported on the plain-SQL backend.
// --------------------------------------------------------------------------

func (s *SQLGraphStore) CypherQuery(_ context.Context, _ string, _ map[string]any) ([]*coreg.Node, error) {
	return nil, coreg.ErrCypherNotSupported
}

func (s *SQLGraphStore) CypherExec(_ context.Context, _ string, _ map[string]any) error {
	return coreg.ErrCypherNotSupported
}

// --------------------------------------------------------------------------
// Lifecycle
// --------------------------------------------------------------------------

// Close is a no-op: the pgxpool is owned by the caller (storage.PgDB), shared
// with the relational stores.
func (s *SQLGraphStore) Close() error {
	return nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// neighborWhere builds a WHERE clause (over graph_edges) selecting the edges
// incident to nodeID in the requested direction, optionally restricted to the
// given edge labels. Placeholders are numbered from $1.
func (s *SQLGraphStore) neighborWhere(nodeID string, direction coreg.Direction, labels []string) (string, []any) {
	var clauses []string
	var args []any
	idx := 1

	switch direction {
	case coreg.Outgoing:
		clauses = append(clauses, fmt.Sprintf("source = $%d", idx))
		args = append(args, nodeID)
		idx++
	case coreg.Incoming:
		clauses = append(clauses, fmt.Sprintf("target = $%d", idx))
		args = append(args, nodeID)
		idx++
	case coreg.Both:
		clauses = append(clauses, fmt.Sprintf("(source = $%d OR target = $%d)", idx, idx+1))
		args = append(args, nodeID, nodeID)
		idx += 2
	}

	if len(labels) > 0 {
		ph := make([]string, len(labels))
		for i, l := range labels {
			ph[i] = fmt.Sprintf("$%d", idx)
			args = append(args, l)
			idx++
		}
		clauses = append(clauses, "label IN ("+strings.Join(ph, ",")+")")
	}

	return strings.Join(clauses, " AND "), args
}

// nodesFromEdges resolves the distinct neighbour nodes reachable across the
// given edges (the endpoint that is not nodeID), preserving first-seen order.
// When scope is non-nil, only edges active at that scope contribute (evaluated
// in Go via Validity.Matches, exactly as the SQLite reference does). Missing
// endpoint nodes are silently skipped, matching the reference.
func (s *SQLGraphStore) nodesFromEdges(ctx context.Context, nodeID string, edges []*coreg.Edge, scope *coreg.Scope) ([]*coreg.Node, error) {
	seen := make(map[string]bool)
	var order []string
	for _, e := range edges {
		if scope != nil && !e.Validity.Matches(*scope) {
			continue
		}
		other := e.Source
		if e.Source == nodeID {
			other = e.Target
		}
		if !seen[other] {
			seen[other] = true
			order = append(order, other)
		}
	}
	var result []*coreg.Node
	for _, id := range order {
		n, err := s.GetNode(ctx, id)
		if err == nil {
			result = append(result, n)
		}
	}
	return result, nil
}

func (s *SQLGraphStore) queryNodes(ctx context.Context, where string, args []any) ([]*coreg.Node, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, label, properties, created_at, updated_at FROM graph_nodes WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*coreg.Node
	for rows.Next() {
		n, err := scanSQLNode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan node row: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (s *SQLGraphStore) queryEdges(ctx context.Context, where string, args []any) ([]*coreg.Edge, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, source, target, label, properties, validity, created_at, updated_at
		 FROM graph_edges WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	var edges []*coreg.Edge
	for rows.Next() {
		e, err := scanSQLEdge(rows)
		if err != nil {
			return nil, fmt.Errorf("scan edge row: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// sqlRow abstracts *pgxpool row and pgx.Rows so scanners work for both single-
// row and multi-row reads.
type sqlRow interface {
	Scan(dest ...any) error
}

func scanSQLNode(row sqlRow) (*coreg.Node, error) {
	var n coreg.Node
	if err := row.Scan(&n.ID, &n.Label, &n.Properties, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, err
	}
	if n.Properties == nil {
		n.Properties = map[string]string{}
	}
	n.CreatedAt = n.CreatedAt.UTC()
	n.UpdatedAt = n.UpdatedAt.UTC()
	return &n, nil
}

func scanSQLEdge(row sqlRow) (*coreg.Edge, error) {
	var e coreg.Edge
	var validityRaw []byte
	if err := row.Scan(&e.ID, &e.Source, &e.Target, &e.Label, &e.Properties, &validityRaw, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	if e.Properties == nil {
		e.Properties = map[string]string{}
	}
	e.Validity = sqlDecodeValidity(validityRaw)
	e.CreatedAt = e.CreatedAt.UTC()
	e.UpdatedAt = e.UpdatedAt.UTC()
	return &e, nil
}

// sqlPropsJSON marshals a property map to a jsonb object literal, normalising
// nil to an empty object so the stored value is always a jsonb object (a jsonb
// `null` would never satisfy the `@>` containment used by FindNodes/FindEdges).
func sqlPropsJSON(props map[string]string) []byte {
	if props == nil {
		props = map[string]string{}
	}
	b, err := json.Marshal(props)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// sqlValidityJSON marshals an edge Validity to jsonb, or nil (SQL NULL) when the
// edge has no validity.
func sqlValidityJSON(v *coreg.Validity) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// sqlDecodeValidity reconstructs a Validity from stored jsonb. It returns nil
// when the column was NULL or carries no bounds/tags — mirroring the SQLite
// reference, which collapses an all-empty validity back to nil on read.
func sqlDecodeValidity(raw []byte) *coreg.Validity {
	if len(raw) == 0 {
		return nil
	}
	var v coreg.Validity
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	if v.ValidFrom == nil && v.ValidTo == nil && len(v.Tags) == 0 {
		return nil
	}
	return &v
}
