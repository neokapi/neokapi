package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	platev "github.com/neokapi/neokapi/platform/event"
)

// RuleStore implements AutomationRuleStore and AutomationHistoryStore.
type RuleStore struct {
	db      *sql.DB
	dialect bstore.Dialect
}

// NewSQLiteRuleStore creates a new SQLite-backed automation rule store.
func NewSQLiteRuleStore(db *sql.DB) *RuleStore {
	return &RuleStore{db: db, dialect: bstore.DialectSQLite}
}

// NewPostgresRuleStore creates a new PostgreSQL-backed automation rule store.
func NewPostgresRuleStore(db *sql.DB) *RuleStore {
	return &RuleStore{db: db, dialect: bstore.DialectPostgres}
}

func (s *RuleStore) q(query string) string {
	return bstore.Rebind(s.dialect, query)
}

func (s *RuleStore) ListRules(ctx context.Context, projectID string) ([]StoredRule, error) {
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT id, project_id, name, trigger, conditions, actions, enabled, builtin, created_at, updated_at
		 FROM automation_rules WHERE project_id = ? ORDER BY created_at`),
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []StoredRule
	for rows.Next() {
		r, err := s.scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, *r)
	}
	return rules, rows.Err()
}

func (s *RuleStore) GetRule(ctx context.Context, id string) (*StoredRule, error) {
	row := s.db.QueryRowContext(ctx, s.q(
		`SELECT id, project_id, name, trigger, conditions, actions, enabled, builtin, created_at, updated_at
		 FROM automation_rules WHERE id = ?`), id)
	return s.scanRule(row)
}

func (s *RuleStore) CreateRule(ctx context.Context, rule *StoredRule) error {
	condJSON, _ := json.Marshal(rule.Conditions)
	actJSON, _ := json.Marshal(rule.Actions)
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := boolToInt(rule.Enabled)
	builtin := boolToInt(rule.Builtin)
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO automation_rules (id, project_id, name, trigger, conditions, actions, enabled, builtin, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		rule.ID, rule.ProjectID, rule.Name, string(rule.Trigger), string(condJSON), string(actJSON),
		enabled, builtin, now, now)
	return err
}

func (s *RuleStore) UpdateRule(ctx context.Context, rule *StoredRule) error {
	condJSON, _ := json.Marshal(rule.Conditions)
	actJSON, _ := json.Marshal(rule.Actions)
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := boolToInt(rule.Enabled)
	_, err := s.db.ExecContext(ctx, s.q(
		`UPDATE automation_rules SET name=?, trigger=?, conditions=?, actions=?, enabled=?, updated_at=?
		 WHERE id=? AND builtin=0`),
		rule.Name, string(rule.Trigger), string(condJSON), string(actJSON), enabled, now, rule.ID)
	return err
}

func (s *RuleStore) DeleteRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`DELETE FROM automation_rules WHERE id=? AND builtin=0`), id)
	return err
}

func (s *RuleStore) ToggleRule(ctx context.Context, id string, enabled bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	e := boolToInt(enabled)
	_, err := s.db.ExecContext(ctx, s.q(
		`UPDATE automation_rules SET enabled=?, updated_at=? WHERE id=?`), e, now, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *RuleStore) RecordExecution(ctx context.Context, entry *HistoryEntry) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO automation_history (id, rule_id, project_id, event_id, status, error, started_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		entry.ID, entry.RuleID, entry.ProjectID, entry.EventID, entry.Status, entry.Error,
		entry.StartedAt.UTC().Format(time.RFC3339),
		entry.EndedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *RuleStore) ListHistory(ctx context.Context, projectID string, limit int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT id, rule_id, project_id, event_id, status, error, started_at, ended_at
		 FROM automation_history WHERE project_id = ? ORDER BY started_at DESC LIMIT ?`),
		projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var startedAt, endedAt string
		if err := rows.Scan(&e.ID, &e.RuleID, &e.ProjectID, &e.EventID, &e.Status, &e.Error, &startedAt, &endedAt); err != nil {
			return nil, err
		}
		e.StartedAt = parseTime(startedAt)
		e.EndedAt = parseTime(endedAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func (s *RuleStore) scanRule(row scanner) (*StoredRule, error) {
	var r StoredRule
	var trigger, condJSON, actJSON, createdAt, updatedAt string
	var enabled, builtin int

	if err := row.Scan(&r.ID, &r.ProjectID, &r.Name, &trigger, &condJSON, &actJSON,
		&enabled, &builtin, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	r.Trigger = platev.EventType(trigger)
	r.Enabled = enabled != 0
	r.Builtin = builtin != 0
	_ = json.Unmarshal([]byte(condJSON), &r.Conditions)
	_ = json.Unmarshal([]byte(actJSON), &r.Actions)
	r.CreatedAt = parseTime(createdAt)
	r.UpdatedAt = parseTime(updatedAt)
	return &r, nil
}

// parseTime parses a timestamp string in various formats.
func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	t, _ := time.Parse("2006-01-02 15:04:05", s)
	return t
}
