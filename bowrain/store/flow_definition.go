package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/id"
)

// ErrFlowDefNotFound is returned when a flow definition does not exist.
var ErrFlowDefNotFound = errors.New("flow definition not found")

// ErrFlowDefExists is returned when a create names an id a stored flow
// already has.
var ErrFlowDefExists = errors.New("a flow definition with this id exists")

// ErrFlowDefChanged is returned when the stored flow no longer has the
// revision a save was based on: someone saved it since.
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

// StoredFlow is a project flow definition as the store holds it: the
// definition and its revision, a token the store replaces on every write.
// A save names the revision it was based on, and the store refuses one made
// against an older read. A token is never reused, so a save based on a flow
// that was deleted and created again under the same id is refused too.
type StoredFlow struct {
	flow.FlowDefinition
	Revision string `json:"revision,omitempty"`
}

// MarshalJSON writes the definition and then the revision. The embedded
// FlowDefinition has its own MarshalJSON, which Go promotes to StoredFlow,
// so without this the revision would be left out.
func (f StoredFlow) MarshalJSON() ([]byte, error) {
	def, err := json.Marshal(f.FlowDefinition)
	if err != nil {
		return nil, err
	}
	if f.Revision == "" {
		return def, nil
	}
	rev, err := json.Marshal(f.Revision)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(def)+len(rev)+13)
	out = append(out, def[:len(def)-1]...)
	out = append(out, `,"revision":`...)
	out = append(out, rev...)
	return append(out, '}'), nil
}

// List returns all project-stored flow definitions, ordered by name.
func (s *FlowDefStore) List(ctx context.Context, projectID string) ([]StoredFlow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, graph, created_at, updated_at, revision
		 FROM flow_definitions WHERE project_id = $1 ORDER BY name`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var defs []StoredFlow
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
func (s *FlowDefStore) Get(ctx context.Context, projectID, id string) (*StoredFlow, error) {
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

// newRevision is a fresh revision token. It never repeats, so no two writes
// of a flow, even across a delete and a create under the same id, share one.
func newRevision() string { return id.New() }

// Create stores a new project flow definition under a fresh revision. It
// never replaces one: an id a stored flow already has is ErrFlowDefExists.
func (s *FlowDefStore) Create(ctx context.Context, projectID string, def *flow.FlowDefinition) error {
	def.Source = "project"
	graph, err := json.Marshal(def)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO flow_definitions (id, project_id, name, description, graph, created_at, updated_at, revision)
		 VALUES ($1, $2, $3, $4, $5, $6, $6, $7)
		 ON CONFLICT (project_id, id) DO NOTHING`,
		def.ID, projectID, def.Name, def.Description, string(graph), now, newRevision())
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

// Update replaces a stored flow definition when it still has the revision the
// save was based on, and gives it a fresh one. The check and the write are one
// statement, so two saves based on the same revision cannot both succeed.
// Update never creates: a flow that is not stored is ErrFlowDefNotFound, and
// one whose revision moved on is ErrFlowDefChanged.
func (s *FlowDefStore) Update(ctx context.Context, projectID string, def *flow.FlowDefinition, base string) error {
	def.Source = "project"
	graph, err := json.Marshal(def)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE flow_definitions
		 SET name = $3, description = $4, graph = $5, updated_at = $6, revision = $8
		 WHERE project_id = $1 AND id = $2 AND revision = $7`,
		projectID, def.ID, def.Name, def.Description, string(graph), now, base, newRevision())
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := s.Get(ctx, projectID, def.ID); err != nil {
		return err
	}
	return ErrFlowDefChanged
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

func scanFlowDef(row scanner) (*StoredFlow, error) {
	var (
		flowID, name, description, graph string
		createdAt, updatedAt             time.Time
		revision                         string
	)
	if err := row.Scan(&flowID, &name, &description, &graph, &createdAt, &updatedAt, &revision); err != nil {
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
	def.ID = flowID
	def.Name = name
	def.Description = description
	def.Source = "project"
	def.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	def.ModifiedAt = updatedAt.UTC().Format(time.RFC3339)
	return &StoredFlow{FlowDefinition: def, Revision: revision}, nil
}
