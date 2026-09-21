package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/storage"
)

// sessionsMigrationsTable is the agent-session table's own migration ledger
// inside `workspace.db`. The project registry, the widened rules, the operation
// log and the context graph each keep one, which is what lets them share the
// file without replaying each other's migrations.
const sessionsMigrationsTable = "workspace_sessions_migrations"

var sessionsMigrations = []storage.Migration{{
	Version:     1,
	Description: "agent sessions working in the workspace",
	SQL: `
CREATE TABLE IF NOT EXISTS workspace_agent_sessions (
    id         TEXT NOT NULL,
    project    TEXT NOT NULL DEFAULT '',
    agent      TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    last_seen  TEXT NOT NULL,
    PRIMARY KEY (id, project)
);
CREATE INDEX IF NOT EXISTS idx_workspace_agent_sessions_seen ON workspace_agent_sessions(last_seen);`,
}}

// AgentSession is one agent at work in a project, as the workspace holds it.
//
// An agent runs in a process of its own and records what it notices under a
// session id. Another process reads these rows to show that somebody is working
// here: the desktop draws it beside the project, and `kapi context log
// --session` reads back what that session recorded.
//
// The row carries no work of its own. Everything the session recorded is in the
// operation log under the same id, so a row that ages out loses nothing.
type AgentSession struct {
	// ID is the session, minted by the process that opened it.
	ID string
	// Project is the project the session is working in. Empty for a session
	// that has not named one.
	Project ProjectKey
	// Agent is the client's own name, as it introduced itself. It may be empty.
	Agent string
	// Started is when the session first said it was here, and LastSeen when it
	// last did. Both in UTC.
	Started  time.Time
	LastSeen time.Time
}

// ErrNoSessionID reports a session note with no id.
var ErrNoSessionID = errors.New("workspace: an agent session needs an id")

// AgentSessionRetention is how long a session note is kept after it was last
// seen. A process that stops never says so, so the row is pruned on the next
// write rather than by anything a person has to run.
const AgentSessionRetention = 7 * 24 * time.Hour

// sessions brings the agent-session schema up to date on first use. The
// registry database is shared, so the migration runs from here rather than from
// Open, and a read-only workspace skips it the way Open skips the registry's.
func (w *Workspace) sessions() (*storage.DB, error) {
	if w.backend.Describe().ReadOnly {
		return w.registry, nil
	}
	w.sessionsOnce.Do(func() {
		w.sessionsErr = storage.Migrate(w.registry, sessionsMigrationsTable, sessionsMigrations)
	})
	if w.sessionsErr != nil {
		return nil, fmt.Errorf("workspace: migrate agent sessions: %w", w.sessionsErr)
	}
	return w.registry, nil
}

// NoteAgentSession records that an agent is at work, and moves its last-seen
// time forward on every later call under the same id and project.
//
// The started time is kept from the first note, so a session that has been
// working for an hour still says when it began. One session working in two
// projects keeps a row per project, because what a surface shows is who is
// working HERE.
func (w *Workspace) NoteAgentSession(ctx context.Context, s AgentSession) error {
	if s.ID == "" {
		return ErrNoSessionID
	}
	if w.backend.Describe().ReadOnly {
		return ErrReadOnly
	}
	db, err := w.sessions()
	if err != nil {
		return err
	}
	seen := s.LastSeen
	if seen.IsZero() {
		seen = time.Now()
	}
	started := s.Started
	if started.IsZero() {
		started = seen
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workspace_agent_sessions (id, project, agent, started_at, last_seen) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id, project) DO UPDATE SET
    agent = CASE WHEN excluded.agent = '' THEN workspace_agent_sessions.agent ELSE excluded.agent END,
    last_seen = excluded.last_seen`,
		s.ID, string(s.Project), s.Agent,
		started.UTC().Format(time.RFC3339Nano), seen.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("workspace: note agent session %s: %w", s.ID, err)
	}
	// Pruning rides on the write so nothing has to be run by hand. A process
	// that exits says nothing about it, and the only evidence it has gone is
	// that its row stopped moving.
	cutoff := seen.Add(-AgentSessionRetention).UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `DELETE FROM workspace_agent_sessions WHERE last_seen < ?`, cutoff); err != nil {
		return fmt.Errorf("workspace: prune agent sessions: %w", err)
	}
	return nil
}

// AgentSessions reports the sessions seen within the last activeWithin, newest
// first. A zero or negative activeWithin reports every session the workspace
// still holds.
//
// A session narrows the listing to one project when project is non-empty.
func (w *Workspace) AgentSessions(ctx context.Context, project ProjectKey, activeWithin time.Duration) ([]AgentSession, error) {
	db, err := w.sessions()
	if err != nil {
		return nil, err
	}
	if w.backend.Describe().ReadOnly && !sessionTableExists(ctx, db) {
		// A workspace opened for reading alone never ran the migration, so
		// there may be no table to read. No sessions is the honest answer.
		return nil, nil
	}
	query := `SELECT id, project, agent, started_at, last_seen FROM workspace_agent_sessions`
	var (
		args  []any
		where []string
	)
	if project != "" {
		where = append(where, `project = ?`)
		args = append(args, string(project))
	}
	if activeWithin > 0 {
		where = append(where, `last_seen >= ?`)
		args = append(args, time.Now().Add(-activeWithin).UTC().Format(time.RFC3339Nano))
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY last_seen DESC, id`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workspace: list agent sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AgentSession
	for rows.Next() {
		var (
			s                 AgentSession
			key               string
			started, lastSeen string
		)
		if err := rows.Scan(&s.ID, &key, &s.Agent, &started, &lastSeen); err != nil {
			return nil, fmt.Errorf("workspace: list agent sessions: %w", err)
		}
		s.Project = ProjectKey(key)
		s.Started = parseSessionTime(started)
		s.LastSeen = parseSessionTime(lastSeen)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workspace: list agent sessions: %w", err)
	}
	return out, nil
}

// sessionTableExists reports whether the agent-session table has been created.
// Only a read-only workspace asks: every writable one runs the migration first.
func sessionTableExists(ctx context.Context, db *storage.DB) bool {
	var name string
	err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'workspace_agent_sessions'`).Scan(&name)
	return err == nil && name != ""
}

// parseSessionTime reads a stored instant back, answering with the zero time
// for anything it cannot read.
func parseSessionTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}
