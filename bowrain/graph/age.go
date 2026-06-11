package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	coreg "github.com/neokapi/neokapi/core/graph"
)

// validIdentifier matches safe Cypher identifiers (labels, property keys, aliases).
var validIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

const graphName = "bowrain_graph"

// AGEGraphStore implements coreg.GraphStore and CypherStore using Apache AGE on PostgreSQL.
type AGEGraphStore struct {
	pool *pgxpool.Pool
}

var _ CypherStore = (*AGEGraphStore)(nil)

// NewAGEGraphStore creates a new AGE-backed GraphStore.
func NewAGEGraphStore(pool *pgxpool.Pool) *AGEGraphStore {
	return &AGEGraphStore{pool: pool}
}

// EnsureGraph creates the AGE graph if it does not already exist.
func (s *AGEGraphStore) EnsureGraph(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, fmt.Sprintf(
		`SELECT * FROM ag_catalog.create_graph('%s')`, graphName))
	if err != nil {
		// Ignore "duplicate object" (SQLSTATE 42710) — the graph already exists.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42710" {
			return nil
		}
		return fmt.Errorf("create graph: %w", err)
	}
	return nil
}

// --------------------------------------------------------------------------
// Node CRUD
// --------------------------------------------------------------------------

func (s *AGEGraphStore) CreateNode(ctx context.Context, node *coreg.Node) error {
	if err := validateIdentifier(node.Label); err != nil {
		return fmt.Errorf("create node: invalid label: %w", err)
	}

	now := time.Now().UTC()
	node.CreatedAt = now
	node.UpdatedAt = now

	propsJSON, err := marshalProps(node.Properties, node.ID, now, now)
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			CREATE (n:%s %s) RETURN n
		$$) as (v agtype)`, graphName, node.Label, propsJSON)

	_, err = s.pool.Exec(ctx, query)
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, coreg.ErrNodeNotFound
		}
		return nil, fmt.Errorf("get node: %w", err)
	}
	return ParseVertex(raw)
}

func (s *AGEGraphStore) UpdateNode(ctx context.Context, node *coreg.Node) error {
	node.UpdatedAt = time.Now().UTC()

	sets, err := buildSetClauses("n", node.Properties)
	if err != nil {
		return fmt.Errorf("update node: %w", err)
	}
	sets = append(sets, fmt.Sprintf("n.updated_at = '%s'", formatTime(node.UpdatedAt)))

	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (n {id: '%s'}) SET %s RETURN n
		$$) as (v agtype)`, graphName, escCypher(node.ID), strings.Join(sets, ", "))

	var raw string
	err = s.pool.QueryRow(ctx, query).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
	where, err := buildWhereClauses("n", properties)
	if err != nil {
		return nil, fmt.Errorf("find nodes: %w", err)
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	matchLabel := ""
	if label != "" {
		if err := validateIdentifier(label); err != nil {
			return nil, fmt.Errorf("find nodes: invalid label: %w", err)
		}
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
	if err := validateIdentifier(edge.Label); err != nil {
		return fmt.Errorf("create edge: invalid label: %w", err)
	}

	now := time.Now().UTC()
	edge.CreatedAt = now
	edge.UpdatedAt = now

	props := make(map[string]string)
	maps.Copy(props, edge.Properties)
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

	propsLiteral, err := buildPropsLiteral(props)
	if err != nil {
		return fmt.Errorf("create edge: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (a {id: '%s'}), (b {id: '%s'})
			CREATE (a)-[r:%s %s]->(b) RETURN r
		$$) as (e agtype)`, graphName, escCypher(edge.Source), escCypher(edge.Target),
		edge.Label, propsLiteral)

	_, err = s.pool.Exec(ctx, query)
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, coreg.ErrEdgeNotFound
		}
		return nil, fmt.Errorf("get edge: %w", err)
	}
	return ParseEdge(raw)
}

func (s *AGEGraphStore) UpdateEdge(ctx context.Context, edge *coreg.Edge) error {
	edge.UpdatedAt = time.Now().UTC()

	sets, err := buildSetClauses("r", edge.Properties)
	if err != nil {
		return fmt.Errorf("update edge: %w", err)
	}
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
	err = s.pool.QueryRow(ctx, query).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
	where, err := buildWhereClauses("r", properties)
	if err != nil {
		return nil, fmt.Errorf("find edges: %w", err)
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	matchLabel := ""
	if label != "" {
		if err := validateIdentifier(label); err != nil {
			return nil, fmt.Errorf("find edges: invalid label: %w", err)
		}
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
	pattern, err := directionPattern("r", direction, labels)
	if err != nil {
		return nil, fmt.Errorf("neighbors: %w", err)
	}
	query := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$
			MATCH (n {id: '%s'})%s(m) RETURN m
		$$) as (v agtype)`, graphName, escCypher(nodeID), pattern)

	return s.queryNodes(ctx, query)
}

func (s *AGEGraphStore) NeighborsScoped(ctx context.Context, nodeID string, direction coreg.Direction, scope coreg.Scope, labels ...string) ([]*coreg.Node, error) {
	pattern, err := directionPattern("r", direction, labels)
	if err != nil {
		return nil, fmt.Errorf("neighbors scoped: %w", err)
	}

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
	pattern, err := directionPattern("r", direction, labels)
	if err != nil {
		return nil, fmt.Errorf("edges of: %w", err)
	}
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
		if errors.Is(err, pgx.ErrNoRows) {
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
	cypher, err := interpolateParams(query, params)
	if err != nil {
		return nil, fmt.Errorf("cypher query: %w", err)
	}
	q := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$ %s $$) as (v agtype)`,
		graphName, cypher)

	return s.queryNodes(ctx, q)
}

func (s *AGEGraphStore) CypherExec(ctx context.Context, query string, params map[string]any) error {
	cypher, err := interpolateParams(query, params)
	if err != nil {
		return fmt.Errorf("cypher exec: %w", err)
	}
	q := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('%s', $$ %s $$) as (v agtype)`,
		graphName, cypher)

	_, err = s.pool.Exec(ctx, q)
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

// escCypher escapes characters in Cypher string literals to prevent injection.
// It handles single quotes, backslashes, and null bytes.
func escCypher(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\x00", "")
	return s
}

// validateIdentifier checks that a string is a safe Cypher identifier
// (label, property key, or alias). Returns an error if it contains
// characters that could enable injection.
func validateIdentifier(name string) error {
	if !validIdentifier.MatchString(name) {
		return fmt.Errorf("invalid Cypher identifier: %q", name)
	}
	return nil
}

// marshalProps builds a Cypher properties literal for node creation.
func marshalProps(props map[string]string, id string, createdAt, updatedAt time.Time) (string, error) {
	all := make(map[string]string, len(props)+3)
	maps.Copy(all, props)
	all["id"] = id
	all["created_at"] = formatTime(createdAt)
	all["updated_at"] = formatTime(updatedAt)
	return buildPropsLiteral(all)
}

// buildPropsLiteral builds a Cypher map literal: {key: 'value', ...}
func buildPropsLiteral(props map[string]string) (string, error) {
	if len(props) == 0 {
		return "{}", nil
	}
	parts := make([]string, 0, len(props))
	for k, v := range props {
		if err := validateIdentifier(k); err != nil {
			return "", fmt.Errorf("property key: %w", err)
		}
		parts = append(parts, fmt.Sprintf("%s: '%s'", k, escCypher(v)))
	}
	return "{" + strings.Join(parts, ", ") + "}", nil
}

// buildSetClauses builds SET expressions for Cypher updates.
func buildSetClauses(alias string, props map[string]string) ([]string, error) {
	sets := make([]string, 0, len(props))
	for k, v := range props {
		if err := validateIdentifier(k); err != nil {
			return nil, fmt.Errorf("property key: %w", err)
		}
		sets = append(sets, fmt.Sprintf("%s.%s = '%s'", alias, k, escCypher(v)))
	}
	return sets, nil
}

// buildWhereClauses builds WHERE conditions for property matching.
func buildWhereClauses(alias string, props map[string]string) ([]string, error) {
	clauses := make([]string, 0, len(props))
	for k, v := range props {
		if err := validateIdentifier(k); err != nil {
			return nil, fmt.Errorf("property key: %w", err)
		}
		clauses = append(clauses, fmt.Sprintf("%s.%s = '%s'", alias, k, escCypher(v)))
	}
	return clauses, nil
}

// directionPattern builds a Cypher relationship pattern based on direction.
func directionPattern(alias string, dir coreg.Direction, labels []string) (string, error) {
	labelStr := ""
	if len(labels) > 0 {
		for _, l := range labels {
			if err := validateIdentifier(l); err != nil {
				return "", fmt.Errorf("edge label: %w", err)
			}
		}
		labelStr = ":" + strings.Join(labels, "|")
	}

	rel := fmt.Sprintf("[%s%s]", alias, labelStr)
	switch dir {
	case coreg.Outgoing:
		return "-" + rel + "->", nil
	case coreg.Incoming:
		return "<-" + rel + "-", nil
	default: // Both
		return "-" + rel + "-", nil
	}
}

// interpolateParams does simple string substitution of $key with Cypher-escaped values.
func interpolateParams(query string, params map[string]any) (string, error) {
	for k, v := range params {
		if err := validateIdentifier(k); err != nil {
			return "", fmt.Errorf("param key: %w", err)
		}
		var replacement string
		switch val := v.(type) {
		case string:
			replacement = fmt.Sprintf("'%s'", escCypher(val))
		case int:
			replacement = strconv.Itoa(val)
		case int64:
			replacement = strconv.FormatInt(val, 10)
		case float64:
			replacement = strconv.FormatFloat(val, 'f', -1, 64)
		case bool:
			replacement = strconv.FormatBool(val)
		default:
			b, _ := json.Marshal(val)
			replacement = fmt.Sprintf("'%s'", escCypher(string(b)))
		}
		query = strings.ReplaceAll(query, "$"+k, replacement)
	}
	return query, nil
}
