package workspace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/storage"
)

// rulesMigrationsTable is the widened-rule table's own migration ledger inside
// `workspace.db`. The project registry, the operation log and the context graph
// each keep one, which is what lets them share the file without replaying each
// other's migrations.
const rulesMigrationsTable = "workspace_rules_migrations"

var rulesMigrations = []storage.Migration{{
	Version:     1,
	Description: "rules widened to the whole workspace",
	SQL: `
CREATE TABLE IF NOT EXISTS workspace_rules (
    id        TEXT PRIMARY KEY,
    kind      TEXT NOT NULL,
    origin    TEXT NOT NULL DEFAULT '',
    payload   BLOB NOT NULL,
    widened_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workspace_rules_kind ON workspace_rules(kind, id);`,
}}

// Rule is one piece of context that answers in every project of the workspace
// rather than in the one whose evidence produced it.
//
// Almost everything a project learns stays in that project: its terms, its
// voice profile and its content memory live in its own context store. A rule a
// person deliberately widened has nowhere there to live, because no single
// project owns it, so it lives here beside the registry.
//
// The workspace holds it as opaque bytes and never reads inside them. What a
// rule means is the business of whatever widened it (core/contextop), which
// keeps the workspace ignorant of term rules, voice profiles and every type
// that will be widened later.
type Rule struct {
	// ID addresses the rule. The caller chooses it and is responsible for its
	// stability; widening the same rule twice under one id replaces it.
	ID string
	// Kind says what sort of rule this is, in the vocabulary of whatever
	// widened it. The workspace uses it only to answer a listing narrowed to
	// one kind.
	Kind string
	// Origin is the project whose evidence produced the rule, kept so
	// provenance survives widening. It may be empty.
	Origin ProjectKey
	// Payload is the rule itself, as JSON.
	Payload []byte
	// At is when it was widened, in UTC. Record leaves it to the caller's
	// clock; a zero value takes the moment of the write.
	At time.Time
}

// ErrNoRuleID reports a widened rule with no id.
var ErrNoRuleID = errors.New("workspace: a widened rule needs an id")

// rules brings the widened-rule schema up to date on first use. The registry
// database is shared, so the migration runs from here rather than from Open,
// and a read-only workspace skips it the way Open skips the registry's.
func (w *Workspace) rules() (*storage.DB, error) {
	if w.backend.Describe().ReadOnly {
		return w.registry, nil
	}
	w.rulesOnce.Do(func() {
		w.rulesErr = storage.Migrate(w.registry, rulesMigrationsTable, rulesMigrations)
	})
	if w.rulesErr != nil {
		return nil, fmt.Errorf("workspace: migrate widened rules: %w", w.rulesErr)
	}
	return w.registry, nil
}

// WidenRule puts a rule in force across the workspace, replacing whatever an
// earlier call under the same id put there.
func (w *Workspace) WidenRule(ctx context.Context, rule Rule) error {
	if rule.ID == "" {
		return ErrNoRuleID
	}
	if w.backend.Describe().ReadOnly {
		return ErrReadOnly
	}
	db, err := w.rules()
	if err != nil {
		return err
	}
	at := rule.At
	if at.IsZero() {
		at = time.Now()
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO workspace_rules (id, kind, origin, payload, widened_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    kind = excluded.kind,
    origin = excluded.origin,
    payload = excluded.payload,
    widened_at = excluded.widened_at`,
		rule.ID, rule.Kind, string(rule.Origin), rule.Payload, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("workspace: widen rule %s: %w", rule.ID, err)
	}
	return nil
}

// WidenedRules returns every rule in force across the workspace, oldest first.
// A non-empty kind narrows the listing to that kind.
func (w *Workspace) WidenedRules(ctx context.Context, kind string) ([]Rule, error) {
	db, err := w.rules()
	if err != nil {
		return nil, err
	}
	query := `SELECT id, kind, origin, payload, widened_at FROM workspace_rules`
	var args []any
	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY widened_at, id`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workspace: list widened rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Rule
	for rows.Next() {
		var (
			rule   Rule
			origin string
			when   string
		)
		if err := rows.Scan(&rule.ID, &rule.Kind, &origin, &rule.Payload, &when); err != nil {
			return nil, fmt.Errorf("workspace: list widened rules: %w", err)
		}
		rule.Origin = ProjectKey(origin)
		if parsed, perr := time.Parse(time.RFC3339Nano, when); perr == nil {
			rule.At = parsed.UTC()
		}
		out = append(out, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workspace: list widened rules: %w", err)
	}
	return out, nil
}

// NarrowRule takes a rule back out of force across the workspace. Narrowing a
// rule the workspace does not hold is not an error, so a caller undoing a
// widening it is unsure of does not have to look first.
func (w *Workspace) NarrowRule(ctx context.Context, id string) error {
	if id == "" {
		return ErrNoRuleID
	}
	if w.backend.Describe().ReadOnly {
		return ErrReadOnly
	}
	db, err := w.rules()
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM workspace_rules WHERE id = ?`, id); err != nil {
		return fmt.Errorf("workspace: narrow rule %s: %w", id, err)
	}
	return nil
}

// Record appends operations to the workspace's log and returns them with the
// sequence numbers the backend assigned.
//
// It is how a subsystem above the workspace writes its own history: the
// workspace records its registrations through the backend directly, and
// core/contextop records every change to a project's context through here.
func (w *Workspace) Record(ctx context.Context, ops ...Op) ([]Op, error) {
	return w.backend.Record(ctx, ops...)
}
