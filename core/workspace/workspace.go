package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/storage"
)

// registryMigrationsTable is the project registry's ledger inside
// `workspace.db`. The operation log and the context graph keep their own, so
// each evolves without replaying the others.
const registryMigrationsTable = "workspace_registry_migrations"

var registryMigrations = []storage.Migration{{
	Version:     1,
	Description: "project registry",
	SQL: `
CREATE TABLE IF NOT EXISTS workspace_projects (
    key         TEXT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    last_active TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS workspace_checkouts (
    project TEXT NOT NULL,
    path    TEXT NOT NULL,
    seen_at TEXT NOT NULL,
    PRIMARY KEY (project, path)
);`,
}}

// Registration is what a workspace records about a project.
type Registration struct {
	// Key identifies the project. It is the recipe's `id:`, or its `name:`
	// where the recipe carries no id.
	Key ProjectKey
	// Name is the label a person reads: the recipe's `name:`. It may be empty
	// and it may change, which is why it is not the key.
	Name string
	// Checkouts are the directories this project has been seen at on THIS
	// machine, normalized and sorted. They are hints for a surface listing
	// projects to open; a checkout that has been moved or deleted stays in the
	// list until something opens the project somewhere else.
	Checkouts []string
	// LastActive is when the project was last opened, in UTC.
	LastActive time.Time
}

// Workspace is an open workspace: the project registry, the context graph, and
// one context store per project, over whichever backend holds them.
//
// It is safe for concurrent use. The handles it returns belong to the backend,
// so a caller reads and writes through them and closes the Workspace, never a
// handle.
type Workspace struct {
	backend  Backend
	registry *storage.DB

	// rulesOnce guards the widened-rule schema, which is brought up to date on
	// the first call that needs it rather than at Open: a workspace whose
	// projects never widened anything pays nothing for the table.
	rulesOnce sync.Once
	rulesErr  error
}

// Open prepares a workspace over a backend: it opens the workspace-wide
// database and brings the project registry's schema up to date.
//
// The Workspace takes ownership of the backend, and Close releases it.
func Open(ctx context.Context, backend Backend) (*Workspace, error) {
	if backend == nil {
		return nil, errors.New("workspace: no backend")
	}
	registry, err := backend.Registry(ctx)
	if err != nil {
		return nil, err
	}
	if !backend.Describe().ReadOnly {
		if err := storage.Migrate(registry, registryMigrationsTable, registryMigrations); err != nil {
			return nil, fmt.Errorf("workspace: migrate project registry: %w", err)
		}
	}
	return &Workspace{backend: backend, registry: registry}, nil
}

// OpenLocal prepares a workspace kept in a directory on this machine.
func OpenLocal(ctx context.Context, root string) (*Workspace, error) {
	return Open(ctx, Local(root))
}

// Describe reports which backend holds this workspace and where.
func (w *Workspace) Describe() Descriptor { return w.backend.Describe() }

// Registry returns the workspace-wide database: the project registry, the
// context graph, and the operation log.
func (w *Workspace) Registry() *storage.DB { return w.registry }

// Context opens a project's context store, creating it on first use.
func (w *Workspace) Context(ctx context.Context, key ProjectKey) (*storage.DB, error) {
	return w.backend.Project(ctx, key)
}

// Ops returns the operations recorded after a sequence number, oldest first.
func (w *Workspace) Ops(ctx context.Context, after int64, limit int) ([]Op, error) {
	return w.backend.Since(ctx, after, limit)
}

// Revision is the position the operation log has reached: the sequence number
// of the last operation recorded, and zero for a workspace nothing has been
// recorded in yet.
//
// It is what an answer reports as the state of the context it was read from,
// so two answers carrying the same revision were read from the same workspace
// state, and a higher one says something has been recorded since.
func (w *Workspace) Revision(ctx context.Context) (int64, error) {
	return w.backend.Head(ctx)
}

// Register records that a project was opened: its key, the display name the
// recipe carries, and the checkout it was opened from. It returns the
// registration as the workspace now holds it.
//
// Calling it again is how a project stays current. The display name is replaced
// (a recipe's `name:` is a label a person edits), the checkout is added to the
// list rather than replacing it, and the last-active time moves forward.
//
// A checkout path is a machine-local hint. It is recorded as
// host.NormalizeCheckoutPath spells it, so a directory reached through a
// symlinked parent and the same directory reached directly are one entry.
func (w *Workspace) Register(ctx context.Context, key ProjectKey, name, checkout string) (Registration, error) {
	if key == "" {
		return Registration{}, ErrNoProjectKey
	}
	if w.backend.Describe().ReadOnly {
		return Registration{}, ErrReadOnly
	}
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)

	tx, err := w.registry.BeginTx(ctx, nil)
	if err != nil {
		return Registration{}, fmt.Errorf("workspace: register %s: %w", key, err)
	}
	defer func() { _ = tx.Rollback() }()

	// What the registry already holds, read before the upsert overwrites it.
	// Opening a project is the most frequent thing that happens to a
	// workspace, and an operation per open would make the log's position move
	// whenever anyone looked at anything. The position is what a retrieval
	// answer reports as the state it was read at, so it has to move when the
	// workspace changed and stay still otherwise.
	var (
		priorName string
		known     bool
	)
	switch err := tx.QueryRowContext(ctx,
		`SELECT name FROM workspace_projects WHERE key = ?`, string(key)).Scan(&priorName); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return Registration{}, fmt.Errorf("workspace: register %s: %w", key, err)
	default:
		known = true
	}
	newCheckout := false
	if checkout != "" {
		var held int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM workspace_checkouts WHERE project = ? AND path = ?`,
			string(key), checkout).Scan(&held); err != nil {
			return Registration{}, fmt.Errorf("workspace: register checkout of %s: %w", key, err)
		}
		newCheckout = held == 0
	}
	moved := !known || newCheckout || (name != "" && name != priorName)

	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_projects (key, name, last_active) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
    name = CASE WHEN excluded.name = '' THEN workspace_projects.name ELSE excluded.name END,
    last_active = excluded.last_active`,
		string(key), name, stamp); err != nil {
		return Registration{}, fmt.Errorf("workspace: register %s: %w", key, err)
	}
	if checkout != "" {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_checkouts (project, path, seen_at) VALUES (?, ?, ?)
ON CONFLICT(project, path) DO UPDATE SET seen_at = excluded.seen_at`,
			string(key), checkout, stamp); err != nil {
			return Registration{}, fmt.Errorf("workspace: register checkout of %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Registration{}, fmt.Errorf("workspace: register %s: %w", key, err)
	}

	if moved {
		payload, err := json.Marshal(struct {
			Name     string `json:"name,omitempty"`
			Checkout string `json:"checkout,omitempty"`
		}{Name: name, Checkout: checkout})
		if err != nil {
			return Registration{}, fmt.Errorf("workspace: describe registration of %s: %w", key, err)
		}
		if _, err := w.backend.Record(ctx, Op{
			Project: key, Kind: OpRegisterProject, Payload: payload, At: now,
		}); err != nil {
			return Registration{}, err
		}
	}

	reg, _, err := w.Lookup(ctx, key)
	return reg, err
}

// Lookup returns one project's registration. ok is false when the workspace
// holds no such project.
func (w *Workspace) Lookup(ctx context.Context, key ProjectKey) (Registration, bool, error) {
	if key == "" {
		return Registration{}, false, ErrNoProjectKey
	}
	regs, err := w.list(ctx, key)
	if err != nil || len(regs) == 0 {
		return Registration{}, false, err
	}
	return regs[0], true, nil
}

// Projects returns every project the workspace holds, most recently active
// first. It is what a surface listing projects reads.
func (w *Workspace) Projects(ctx context.Context) ([]Registration, error) {
	return w.list(ctx, "")
}

// list reads registrations, for one key or for all of them.
func (w *Workspace) list(ctx context.Context, only ProjectKey) ([]Registration, error) {
	query := `SELECT key, name, last_active FROM workspace_projects`
	var args []any
	if only != "" {
		query += ` WHERE key = ?`
		args = append(args, string(only))
	}
	query += ` ORDER BY last_active DESC, key`

	rows, err := w.registry.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workspace: list projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Registration
	index := map[ProjectKey]int{}
	for rows.Next() {
		var (
			reg  Registration
			key  string
			when string
		)
		if err := rows.Scan(&key, &reg.Name, &when); err != nil {
			return nil, fmt.Errorf("workspace: list projects: %w", err)
		}
		reg.Key = ProjectKey(key)
		if parsed, perr := time.Parse(time.RFC3339Nano, when); perr == nil {
			reg.LastActive = parsed.UTC()
		}
		index[reg.Key] = len(out)
		out = append(out, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workspace: list projects: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	checkouts, err := w.checkouts(ctx, only)
	if err != nil {
		return nil, err
	}
	for key, paths := range checkouts {
		if i, ok := index[key]; ok {
			sort.Strings(paths)
			out[i].Checkouts = paths
		}
	}
	return out, nil
}

// checkouts reads the recorded checkout paths, for one project or for all of
// them.
func (w *Workspace) checkouts(ctx context.Context, only ProjectKey) (map[ProjectKey][]string, error) {
	query := `SELECT project, path FROM workspace_checkouts`
	var args []any
	if only != "" {
		query += ` WHERE project = ?`
		args = append(args, string(only))
	}
	rows, err := w.registry.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workspace: list checkouts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[ProjectKey][]string{}
	for rows.Next() {
		var project, path string
		if err := rows.Scan(&project, &path); err != nil {
			return nil, fmt.Errorf("workspace: list checkouts: %w", err)
		}
		out[ProjectKey(project)] = append(out[ProjectKey(project)], path)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workspace: list checkouts: %w", err)
	}
	return out, nil
}

// Close releases the backend and every handle it opened.
func (w *Workspace) Close() error { return w.backend.Close() }
