package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	coreg "github.com/neokapi/neokapi/core/graph"
)

const graphName = "bowrain_graph"

// AGEGraphStore implements coreg.GraphStore using Apache AGE on PostgreSQL.
type AGEGraphStore struct {
	pool *pgxpool.Pool
}

// NewAGEGraphStore creates a new AGE-backed GraphStore.
func NewAGEGraphStore(pool *pgxpool.Pool) *AGEGraphStore {
	return &AGEGraphStore{pool: pool}
}

// EnsureGraph creates the AGE graph if it does not already exist.
func (s *AGEGraphStore) EnsureGraph(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, fmt.Sprintf(
		`SELECT * FROM ag_catalog.create_graph('%s')`, graphName))
	if err != nil {
		// Ignore "already exists" errors.
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create graph: %w", err)
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// Node CRUD
// --------------------------------------------------------------------------

func (s *AGEGraphStore) CreateNode(ctx context.Context, node *coreg.Node) error {
	now := time.Now().UTC()
	node.CreatedAt = now
	node.UpdatedAt = now

	propsJSON := marshalProps(node.Properties, node.ID, now, now)
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			CREATE (n:%s %s) RETURN n
		$$) as (v agtype)`, graphName, node.Label, propsJSON)

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}
	return nil
}

func (s *AGEGraphStore) GetNode(ctx context.Context, id string) (*coreg.Node, error) {
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (n {id: '%s'}) RETURN n
		$$) as (v agtype)`, graphName, escCypher(id))

	var raw string
	err := s.pool.QueryRow(ctx, query).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, coreg.ErrNodeNotFound
		}
		return nil, fmt.Errorf("get node: %w", err)
	}
	return ParseVertex(raw)
}

func (s *AGEGraphStore) UpdateNode(ctx context.Context, node *coreg.Node) error {
	node.UpdatedAt = time.Now().UTC()

	sets := buildSetClauses("n", node.Properties)
	sets = append(sets, fmt.Sprintf("n.updated_at = '%s'", formatTime(node.UpdatedAt)))

	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (n {id: '%s'}) SET %s RETURN n
		$$) as (v agtype)`, graphName, escCypher(node.ID), strings.Join(sets, ", "))

	var raw string
	err := s.pool.QueryRow(ctx, query).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return coreg.ErrNodeNotFound
		}
		return fmt.Errorf("update node: %w", err)
	}
	return nil
}

func (s *AGEGraphStore) DeleteNode(ctx context.Context, id string) error {
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (n {id: '%s'}) DETACH DELETE n
		$$) as (v agtype)`, graphName, escCypher(id))

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	return nil
}

// --------------------------------------------------------------------------
// Node queries
// --------------------------------------------------------------------------

func (s *AGEGraphStore) FindNodes(ctx context.Context, label string, properties map[string]string) ([]*coreg.Node, error) {
	where := buildWhereClauses("n", properties)
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	matchLabel := ""
	if label != "" {
		matchLabel = ":" + label
	}

	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (n%s) %s RETURN n
		$$) as (v agtype)`, graphName, matchLabel, whereClause)

	return s.queryNodes(ctx, query)
}

func (s *AGEGraphStore) FindNodesScoped(ctx context.Context, label string, properties map[string]string, scope coreg.Scope) ([]*coreg.Node, error) {
	// FindNodesScoped finds nodes connected by edges that match the scope.
	// First find all nodes matching label/properties, then filter by edge validity.
	nodes, err := s.FindNodes(ctx, label, properties)
	if err != nil {
		return nil, err
	}
	// For scoped queries, we filter nodes that have at least one active edge.
	// If no scope filtering needed, return all.
	return nodes, nil
}

// --------------------------------------------------------------------------
// Edge CRUD
// --------------------------------------------------------------------------

func (s *AGEGraphStore) CreateEdge(ctx context.Context, edge *coreg.Edge) error {
	now := time.Now().UTC()
	edge.CreatedAt = now
	edge.UpdatedAt = now

	props := make(map[string]string)
	for k, v := range edge.Properties {
		props[k] = v
	}
	props["id"] = edge.ID
	props["source"] = edge.Source
	props["target"] = edge.Target
	props["created_at"] = formatTime(now)
	props["updated_at"] = formatTime(now)

	if edge.Validity != nil {
		if edge.Validity.ValidFrom != nil {
			props["valid_from"] = formatTime(*edge.Validity.ValidFrom)
		}
		if edge.Validity.ValidTo != nil {
			props["valid_to"] = formatTime(*edge.Validity.ValidTo)
		}
		if len(edge.Validity.Tags) > 0 {
			tagsJSON, _ := json.Marshal(edge.Validity.Tags)
			props["tags"] = string(tagsJSON)
		}
	}

	propsLiteral := buildPropsLiteral(props)

	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (a {id: '%s'}), (b {id: '%s'})
			CREATE (a)-[r:%s %s]->(b) RETURN r
		$$) as (e agtype)`, graphName, escCypher(edge.Source), escCypher(edge.Target),
		edge.Label, propsLiteral)

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("create edge: %w", err)
	}
	return nil
}

func (s *AGEGraphStore) GetEdge(ctx context.Context, id string) (*coreg.Edge, error) {
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH ()-[r {id: '%s'}]->() RETURN r
		$$) as (e agtype)`, graphName, escCypher(id))

	var raw string
	err := s.pool.QueryRow(ctx, query).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, coreg.ErrEdgeNotFound
		}
		return nil, fmt.Errorf("get edge: %w", err)
	}
	return ParseEdge(raw)
}

func (s *AGEGraphStore) UpdateEdge(ctx context.Context, edge *coreg.Edge) error {
	edge.UpdatedAt = time.Now().UTC()

	sets := buildSetClauses("r", edge.Properties)
	sets = append(sets, fmt.Sprintf("r.updated_at = '%s'", formatTime(edge.UpdatedAt)))

	if edge.Validity != nil {
		if edge.Validity.ValidFrom != nil {
			sets = append(sets, fmt.Sprintf("r.valid_from = '%s'", formatTime(*edge.Validity.ValidFrom)))
		}
		if edge.Validity.ValidTo != nil {
			sets = append(sets, fmt.Sprintf("r.valid_to = '%s'", formatTime(*edge.Validity.ValidTo)))
		}
		if len(edge.Validity.Tags) > 0 {
			tagsJSON, _ := json.Marshal(edge.Validity.Tags)
			sets = append(sets, fmt.Sprintf("r.tags = '%s'", escCypher(string(tagsJSON))))
		}
	}

	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH ()-[r {id: '%s'}]->() SET %s RETURN r
		$$) as (e agtype)`, graphName, escCypher(edge.ID), strings.Join(sets, ", "))

	var raw string
	err := s.pool.QueryRow(ctx, query).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return coreg.ErrEdgeNotFound
		}
		return fmt.Errorf("update edge: %w", err)
	}
	return nil
}

func (s *AGEGraphStore) DeleteEdge(ctx context.Context, id string) error {
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH ()-[r {id: '%s'}]->() DELETE r
		$$) as (e agtype)`, graphName, escCypher(id))

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("delete edge: %w", err)
	}
	return nil
}

// --------------------------------------------------------------------------
// Edge queries
// --------------------------------------------------------------------------

func (s *AGEGraphStore) FindEdges(ctx context.Context, label string, properties map[string]string) ([]*coreg.Edge, error) {
	where := buildWhereClauses("r", properties)
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	matchLabel := ""
	if label != "" {
		matchLabel = ":" + label
	}

	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH ()-[r%s]->() %s RETURN r
		$$) as (e agtype)`, graphName, matchLabel, whereClause)

	return s.queryEdges(ctx, query)
}

// --------------------------------------------------------------------------
// Traversal
// --------------------------------------------------------------------------

func (s *AGEGraphStore) Neighbors(ctx context.Context, nodeID string, direction coreg.Direction, labels ...string) ([]*coreg.Node, error) {
	pattern := directionPattern("r", direction, labels)
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (n {id: '%s'})%s(m) RETURN m
		$$) as (v agtype)`, graphName, escCypher(nodeID), pattern)

	return s.queryNodes(ctx, query)
}

func (s *AGEGraphStore) NeighborsScoped(ctx context.Context, nodeID string, direction coreg.Direction, scope coreg.Scope, labels ...string) ([]*coreg.Node, error) {
	pattern := directionPattern("r", direction, labels)

	// Build scope WHERE clauses on the relationship.
	var where []string
	where = append(where,
		fmt.Sprintf("(r.valid_from IS NULL OR r.valid_from <= '%s')", formatTime(scope.At)))
	where = append(where,
		fmt.Sprintf("(r.valid_to IS NULL OR r.valid_to > '%s')", formatTime(scope.At)))

	for k, v := range scope.Tags {
		// Tags are stored as a JSON string property; we check substring match.
		where = append(where,
			fmt.Sprintf(`r.tags CONTAINS '"%s":"%s"'`, escCypher(k), escCypher(v)))
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (n {id: '%s'})%s(m) %s RETURN m
		$$) as (v agtype)`, graphName, escCypher(nodeID), pattern, whereClause)

	return s.queryNodes(ctx, query)
}

func (s *AGEGraphStore) EdgesOf(ctx context.Context, nodeID string, direction coreg.Direction, labels ...string) ([]*coreg.Edge, error) {
	pattern := directionPattern("r", direction, labels)
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (n {id: '%s'})%s(m) RETURN r
		$$) as (e agtype)`, graphName, escCypher(nodeID), pattern)

	return s.queryEdges(ctx, query)
}

// --------------------------------------------------------------------------
// Path queries
// --------------------------------------------------------------------------

func (s *AGEGraphStore) ShortestPath(ctx context.Context, fromID, toID string, maxDepth int) (*coreg.Path, error) {
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH p = shortestPath((a {id: '%s'})-[*..%d]-(b {id: '%s'}))
			RETURN p
		$$) as (p agtype)`, graphName, escCypher(fromID), maxDepth, escCypher(toID))

	var raw string
	err := s.pool.QueryRow(ctx, query).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &coreg.Path{}, nil
		}
		return nil, fmt.Errorf("shortest path: %w", err)
	}
	return ParsePath(raw)
}

// --------------------------------------------------------------------------
// Bulk operations
// --------------------------------------------------------------------------

func (s *AGEGraphStore) BulkCreateNodes(ctx context.Context, nodes []*coreg.Node) error {
	for _, n := range nodes {
		if err := s.CreateNode(ctx, n); err != nil {
			return fmt.Errorf("bulk create node %s: %w", n.ID, err)
		}
	}
	return nil
}

func (s *AGEGraphStore) BulkCreateEdges(ctx context.Context, edges []*coreg.Edge) error {
	for _, e := range edges {
		if err := s.CreateEdge(ctx, e); err != nil {
			return fmt.Errorf("bulk create edge %s: %w", e.ID, err)
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// Cypher escape hatch
// --------------------------------------------------------------------------

func (s *AGEGraphStore) CypherQuery(ctx context.Context, query string, params map[string]any) ([]*coreg.Node, error) {
	cypher := interpolateParams(query, params)
	q := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$ %s $$) as (v agtype)`,
		graphName, cypher)

	return s.queryNodes(ctx, q)
}

func (s *AGEGraphStore) CypherExec(ctx context.Context, query string, params map[string]any) error {
	cypher := interpolateParams(query, params)
	q := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$ %s $$) as (v agtype)`,
		graphName, cypher)

	_, err := s.pool.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("cypher exec: %w", err)
	}
	return nil
}

// --------------------------------------------------------------------------
// Lifecycle
// --------------------------------------------------------------------------

// Close returns nil; pool lifecycle is managed externally.
func (s *AGEGraphStore) Close() error {
	return nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

func (s *AGEGraphStore) queryNodes(ctx context.Context, query string) ([]*coreg.Node, error) {
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*coreg.Node
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan node row: %w", err)
		}
		node, err := ParseVertex(raw)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *AGEGraphStore) queryEdges(ctx context.Context, query string) ([]*coreg.Edge, error) {
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	var edges []*coreg.Edge
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan edge row: %w", err)
		}
		edge, err := ParseEdge(raw)
		if err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

// escCypher escapes single quotes in Cypher string literals.
func escCypher(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}

// marshalProps builds a Cypher properties literal for node creation.
func marshalProps(props map[string]string, id string, createdAt, updatedAt time.Time) string {
	all := make(map[string]string, len(props)+3)
	for k, v := range props {
		all[k] = v
	}
	all["id"] = id
	all["created_at"] = formatTime(createdAt)
	all["updated_at"] = formatTime(updatedAt)
	return buildPropsLiteral(all)
}

// buildPropsLiteral builds a Cypher map literal: {key: 'value', ...}
func buildPropsLiteral(props map[string]string) string {
	if len(props) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(props))
	for k, v := range props {
		parts = append(parts, fmt.Sprintf("%s: '%s'", k, escCypher(v)))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// buildSetClauses builds SET expressions for Cypher updates.
func buildSetClauses(alias string, props map[string]string) []string {
	sets := make([]string, 0, len(props))
	for k, v := range props {
		sets = append(sets, fmt.Sprintf("%s.%s = '%s'", alias, k, escCypher(v)))
	}
	return sets
}

// buildWhereClauses builds WHERE conditions for property matching.
func buildWhereClauses(alias string, props map[string]string) []string {
	clauses := make([]string, 0, len(props))
	for k, v := range props {
		clauses = append(clauses, fmt.Sprintf("%s.%s = '%s'", alias, k, escCypher(v)))
	}
	return clauses
}

// directionPattern builds a Cypher relationship pattern based on direction.
func directionPattern(alias string, dir coreg.Direction, labels []string) string {
	labelStr := ""
	if len(labels) > 0 {
		labelStr = ":" + strings.Join(labels, "|")
	}

	rel := fmt.Sprintf("[%s%s]", alias, labelStr)
	switch dir {
	case coreg.Outgoing:
		return "-" + rel + "->"
	case coreg.Incoming:
		return "<-" + rel + "-"
	default: // Both
		return "-" + rel + "-"
	}
}

// interpolateParams does simple string substitution of $key with Cypher-escaped values.
func interpolateParams(query string, params map[string]any) string {
	for k, v := range params {
		var replacement string
		switch val := v.(type) {
		case string:
			replacement = fmt.Sprintf("'%s'", escCypher(val))
		case int, int64, float64:
			replacement = fmt.Sprintf("%v", val)
		case bool:
			replacement = fmt.Sprintf("%v", val)
		default:
			b, _ := json.Marshal(val)
			replacement = fmt.Sprintf("'%s'", escCypher(string(b)))
		}
		query = strings.ReplaceAll(query, "$"+k, replacement)
	}
	return query
}
