package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/storage"
)

// RuleStore implements AutomationRuleStore and AutomationHistoryStore.
// PostgreSQL-only — the server and worker always use PostgreSQL.
type RuleStore struct {
	db *sql.DB
}

// NewRuleStore creates an automation rule store.
func NewRuleStore(db *sql.DB) *RuleStore {
	return &RuleStore{db: db}
}

func (s *RuleStore) ListRules(ctx context.Context, projectID string) ([]StoredRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, trigger, conditions, actions, enabled, builtin, created_at, updated_at
		 FROM automation_rules WHERE project_id = $1 ORDER BY created_at`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []StoredRule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, *r)
	}
	return rules, rows.Err()
}

func (s *RuleStore) GetRule(ctx context.Context, id string) (*StoredRule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, trigger, conditions, actions, enabled, builtin, created_at, updated_at
		 FROM automation_rules WHERE id = $1`, id)
	return scanRule(row)
}

func (s *RuleStore) CreateRule(ctx context.Context, rule *StoredRule) error {
	condJSON, _ := json.Marshal(rule.Conditions)
	actJSON, _ := json.Marshal(rule.Actions)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO automation_rules (id, project_id, name, trigger, conditions, actions, enabled, builtin, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		rule.ID, rule.ProjectID, rule.Name, string(rule.Trigger), string(condJSON), string(actJSON),
		rule.Enabled, rule.Builtin, now, now)
	return err
}

func (s *RuleStore) UpdateRule(ctx context.Context, rule *StoredRule) error {
	condJSON, _ := json.Marshal(rule.Conditions)
	actJSON, _ := json.Marshal(rule.Actions)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_rules SET name=$1, trigger=$2, conditions=$3, actions=$4, enabled=$5, updated_at=$6
		 WHERE id=$7 AND builtin=FALSE`,
		rule.Name, string(rule.Trigger), string(condJSON), string(actJSON), rule.Enabled, now, rule.ID)
	return err
}

func (s *RuleStore) DeleteRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM automation_rules WHERE id=$1 AND builtin=FALSE`, id)
	return err
}

func (s *RuleStore) ToggleRule(ctx context.Context, id string, enabled bool) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_rules SET enabled=$1, updated_at=$2 WHERE id=$3`, enabled, now, id)
	return err
}

func (s *RuleStore) RecordExecution(ctx context.Context, entry *HistoryEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO automation_history (id, rule_id, project_id, event_id, status, error, started_at, ended_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		entry.ID, entry.RuleID, entry.ProjectID, entry.EventID, entry.Status, entry.Error,
		entry.StartedAt, entry.EndedAt)
	return err
}

// maxHistoryLimit bounds one page of automation history.
const maxHistoryLimit = 200

// ListHistory returns one page of a project's execution history, newest first.
//
// The cursor is the (started_at, id) tuple of the last row on the page, not the
// timestamp alone: one event fans out to every matching rule within the same
// microsecond, so a `started_at < cursor` seek would drop every sibling of the
// row the page ended on.
func (s *RuleStore) ListHistory(ctx context.Context, q HistoryQuery) (*HistoryPage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}

	// Constant skeleton + $N placeholder tokens only; every value binds
	// through args.
	const skeleton = `SELECT id, rule_id, project_id, event_id, status, error, started_at, ended_at
		 FROM automation_history WHERE project_id = $1%s
		 ORDER BY started_at DESC, id DESC LIMIT $%d`
	args := []any{q.ProjectID}
	seek := ""
	if at, entryID, ok := parseHistoryCursor(q.Cursor); ok {
		seek = " AND (started_at, id) < ($2, $3)"
		args = append(args, at, entryID)
	}
	query := fmt.Sprintf(skeleton, seek, len(args)+1)
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.RuleID, &e.ProjectID, &e.EventID, &e.Status, &e.Error, &e.StartedAt, &e.EndedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	page := &HistoryPage{Entries: entries}
	if len(entries) > limit {
		page.Entries = entries[:limit]
		last := page.Entries[limit-1]
		page.NextCursor = last.StartedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID
	}
	return page, nil
}

// parseHistoryCursor splits a "<started_at>|<id>" cursor. An unparseable cursor
// reads as absent, so a stale client gets the first page rather than an error.
func parseHistoryCursor(cursor string) (time.Time, string, bool) {
	ts, entryID, found := strings.Cut(cursor, "|")
	if !found || entryID == "" {
		return time.Time{}, "", false
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", false
	}
	return at, entryID, true
}

// scanner is an alias for storage.Scanner, satisfied by *sql.Row and *sql.Rows.
type scanner = storage.Scanner

func scanRule(row scanner) (*StoredRule, error) {
	var r StoredRule
	var trigger, condJSON, actJSON string

	if err := row.Scan(&r.ID, &r.ProjectID, &r.Name, &trigger, &condJSON, &actJSON,
		&r.Enabled, &r.Builtin, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}

	r.Trigger = platev.EventType(trigger)
	// Conditions and actions are the rule itself. A rule whose conditions
	// silently decode to nil matches everything it was meant to filter, and one
	// whose actions decode to nil does nothing while still reporting as active
	// — both indistinguishable from a rule deliberately written that way. The
	// columns are NOT NULL DEFAULT '[]' but permit an empty string, so an empty
	// value is an old row rather than corruption.
	if condJSON != "" {
		if err := json.Unmarshal([]byte(condJSON), &r.Conditions); err != nil {
			return nil, fmt.Errorf("rule %s: unmarshal conditions: %w", r.ID, err)
		}
	}
	if actJSON != "" {
		if err := json.Unmarshal([]byte(actJSON), &r.Actions); err != nil {
			return nil, fmt.Errorf("rule %s: unmarshal actions: %w", r.ID, err)
		}
	}
	return &r, nil
}
