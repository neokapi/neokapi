package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/flow"
)

// ErrFlowDefNotFound is returned when a flow definition does not exist.
var ErrFlowDefNotFound = errors.New("flow definition not found")

// ErrFlowDefExists is returned when a create names an id a stored flow
// already has.
var ErrFlowDefExists = errors.New("a flow definition with this id exists")

// ErrFlowDefChanged is returned when a save names the revision it was based
// on and the stored flow no longer has it: someone saved it since.
var ErrFlowDefChanged = errors.New("flow definition changed since it was read")

// FlowDefStore persists project-scoped flow definitions (Bowrain AD-013).
// The server and worker always use PostgreSQL, so the store is PostgreSQL only.
//
// Built-in flows (flowdef.BuiltInFlows) live in code and are merged in at the
// API layer, so automation run_flow actions can reference either a built-in
// flow id or a project-stored flow id by the same name field.
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
		`SELECT id, name, description, graph, created_at, updated_at, revision
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
		`SELECT id, name, description, graph, created_at, updated_at, revision
		 FROM flow_definitions WHERE project_id = $1 AND id = $2`,
		projectID, id)
	def, err := scanFlowDef(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFlowDefNotFound
	}
	return def, err
}

// Upsert creates or replaces a project flow definition. The definition's
// Source is forced to "project" and timestamps are managed by the store. Each
// save raises the stored revision by one.
func (s *FlowDefStore) Upsert(ctx context.Context, projectID string, def *flow.FlowDefinition) error {
	return s.Save(ctx, projectID, def, 0)
}

// Save creates or replaces a project flow definition, as Upsert does, unless
// base names the revision the save was based on (base > 0) and the stored flow
// no longer has it. Then nothing is written and the error is
// ErrFlowDefChanged. A save based on a revision of a flow that is gone is
// refused the same way. The check and the write are one statement, so two
// saves based on the same revision cannot both succeed.
func (s *FlowDefStore) Save(ctx context.Context, projectID string, def *flow.FlowDefinition, base int64) error {
	def.Source = "project"
	// The revision is the store's, kept in its own column.
	def.Revision = 0
	graph, err := json.Marshal(def)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if base <= 0 {
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO flow_definitions (id, project_id, name, description, graph, created_at, updated_at, revision)
			 VALUES ($1, $2, $3, $4, $5, $6, $6, 1)
			 ON CONFLICT (project_id, id)
			 DO UPDATE SET name = $3, description = $4, graph = $5, updated_at = $6,
			   revision = flow_definitions.revision + 1`,
			def.ID, projectID, def.Name, def.Description, string(graph), now)
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE flow_definitions
		 SET name = $3, description = $4, graph = $5, updated_at = $6, revision = revision + 1
		 WHERE project_id = $1 AND id = $2 AND revision = $7`,
		projectID, def.ID, def.Name, def.Description, string(graph), now, base)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrFlowDefChanged
	}
	return nil
}

// Create stores a new project flow definition at revision 1. It never
// replaces one: an id a stored flow already has is ErrFlowDefExists.
func (s *FlowDefStore) Create(ctx context.Context, projectID string, def *flow.FlowDefinition) error {
	def.Source = "project"
	def.Revision = 0
	graph, err := json.Marshal(def)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO flow_definitions (id, project_id, name, description, graph, created_at, updated_at, revision)
		 VALUES ($1, $2, $3, $4, $5, $6, $6, 1)
		 ON CONFLICT (project_id, id) DO NOTHING`,
		def.ID, projectID, def.Name, def.Description, string(graph), now)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrFlowDefExists
	}
	return nil
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
		revision                     int64
	)
	if err := row.Scan(&id, &name, &description, &graph, &createdAt, &updatedAt, &revision); err != nil {
		return nil, err
	}
	var def flow.FlowDefinition
	// The graph column is the flow definition, not metadata about it. Unlike
	// the format probes elsewhere in the tree there is no strict parse behind
	// this one to surface the failure, so a discarded error yielded a flow with
	// no nodes and no edges that read as an empty flow.
	if graph != "" && graph != "{}" {
		if err := json.Unmarshal([]byte(graph), &def); err != nil {
			return nil, fmt.Errorf("unmarshal flow definition graph: %w", err)
		}
	}
	// Authoritative columns win over whatever was embedded in graph JSON.
	def.ID = id
	def.Name = name
	def.Description = description
	def.Source = "project"
	def.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	def.ModifiedAt = updatedAt.UTC().Format(time.RFC3339)
	def.Revision = revision
	return &def, nil
}
