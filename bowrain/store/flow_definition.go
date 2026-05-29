package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/neokapi/neokapi/core/flow"
)

// ErrFlowDefNotFound is returned when a flow definition does not exist.
var ErrFlowDefNotFound = errors.New("flow definition not found")

// FlowDefStore persists project-scoped flow definitions (Bowrain AD-013).
// PostgreSQL-only — the server and worker always use PostgreSQL.
//
// Built-in flows (flow.BuiltInFlows) are not stored here; they are merged in
// at the API layer so automation run_flow actions can reference either a
// built-in flow id or a project-stored flow id by the same name field.
type FlowDefStore struct {
	db *sql.DB
}

// NewFlowDefStore creates a project flow-definition store.
func NewFlowDefStore(db *sql.DB) *FlowDefStore {
	return &FlowDefStore{db: db}
}

// List returns all project-stored flow definitions, ordered by name.
func (s *FlowDefStore) List(ctx context.Context, projectID string) ([]flow.FlowDefinition, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, graph, created_at, updated_at
		 FROM flow_definitions WHERE project_id = $1 ORDER BY name`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var defs []flow.FlowDefinition
	for rows.Next() {
		def, err := scanFlowDef(rows)
		if err != nil {
			return nil, err
		}
		defs = append(defs, *def)
	}
	return defs, rows.Err()
}

// Get returns a single project-stored flow definition by id.
func (s *FlowDefStore) Get(ctx context.Context, projectID, id string) (*flow.FlowDefinition, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, graph, created_at, updated_at
		 FROM flow_definitions WHERE project_id = $1 AND id = $2`,
		projectID, id)
	def, err := scanFlowDef(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFlowDefNotFound
	}
	return def, err
}

// Upsert creates or replaces a project flow definition. The definition's
// Source is forced to "project" and timestamps are managed by the store.
func (s *FlowDefStore) Upsert(ctx context.Context, projectID string, def *flow.FlowDefinition) error {
	def.Source = "project"
	graph, err := json.Marshal(def)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO flow_definitions (id, project_id, name, description, graph, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)
		 ON CONFLICT (project_id, id)
		 DO UPDATE SET name = $3, description = $4, graph = $5, updated_at = $6`,
		def.ID, projectID, def.Name, def.Description, string(graph), now)
	return err
}

// Delete removes a project flow definition. Returns ErrFlowDefNotFound when
// nothing was deleted.
func (s *FlowDefStore) Delete(ctx context.Context, projectID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM flow_definitions WHERE project_id = $1 AND id = $2`, projectID, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrFlowDefNotFound
	}
	return err
}

func scanFlowDef(row scanner) (*flow.FlowDefinition, error) {
	var (
		id, name, description, graph string
		createdAt, updatedAt         time.Time
	)
	if err := row.Scan(&id, &name, &description, &graph, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var def flow.FlowDefinition
	if graph != "" && graph != "{}" {
		_ = json.Unmarshal([]byte(graph), &def)
	}
	// Authoritative columns win over whatever was embedded in graph JSON.
	def.ID = id
	def.Name = name
	def.Description = description
	def.Source = "project"
	def.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	def.ModifiedAt = updatedAt.UTC().Format(time.RFC3339)
	return &def, nil
}
