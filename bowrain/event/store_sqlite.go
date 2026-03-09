package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	platev "github.com/gokapi/gokapi/platform/event"
)

// SQLiteRuleStore implements AutomationRuleStore and AutomationHistoryStore using SQLite.
type SQLiteRuleStore struct {
	db *sql.DB
}

// NewSQLiteRuleStore creates a new SQLite-backed automation rule store.
func NewSQLiteRuleStore(db *sql.DB) *SQLiteRuleStore {
	return &SQLiteRuleStore{db: db}
}

func (s *SQLiteRuleStore) ListRules(ctx context.Context, projectID string) ([]StoredRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, trigger, conditions, actions, enabled, builtin, created_at, updated_at
		 FROM automation_rules WHERE project_id = ? ORDER BY created_at`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []StoredRule
	for rows.Next() {
		var r StoredRule
		var trigger, condJSON, actJSON, createdAt, updatedAt string
		var enabled, builtin int
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Name, &trigger, &condJSON, &actJSON, &enabled, &builtin, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		r.Trigger = platev.EventType(trigger)
		r.Enabled = enabled != 0
		r.Builtin = builtin != 0
		_ = json.Unmarshal([]byte(condJSON), &r.Conditions)
		_ = json.Unmarshal([]byte(actJSON), &r.Actions)
		r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		r.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *SQLiteRuleStore) GetRule(ctx context.Context, id string) (*StoredRule, error) {
	var r StoredRule
	var trigger, condJSON, actJSON, createdAt, updatedAt string
	var enabled, builtin int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, trigger, conditions, actions, enabled, builtin, created_at, updated_at
		 FROM automation_rules WHERE id = ?`, id).
		Scan(&r.ID, &r.ProjectID, &r.Name, &trigger, &condJSON, &actJSON, &enabled, &builtin, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	r.Trigger = platev.EventType(trigger)
	r.Enabled = enabled != 0
	r.Builtin = builtin != 0
	_ = json.Unmarshal([]byte(condJSON), &r.Conditions)
	_ = json.Unmarshal([]byte(actJSON), &r.Actions)
	r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	r.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &r, nil
}

func (s *SQLiteRuleStore) CreateRule(ctx context.Context, rule *StoredRule) error {
	condJSON, _ := json.Marshal(rule.Conditions)
	actJSON, _ := json.Marshal(rule.Actions)
	enabled := 0
	if rule.Enabled {
		enabled = 1
	}
	builtin := 0
	if rule.Builtin {
		builtin = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO automation_rules (id, project_id, name, trigger, conditions, actions, enabled, builtin, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		rule.ID, rule.ProjectID, rule.Name, string(rule.Trigger), string(condJSON), string(actJSON), enabled, builtin)
	return err
}

func (s *SQLiteRuleStore) UpdateRule(ctx context.Context, rule *StoredRule) error {
	condJSON, _ := json.Marshal(rule.Conditions)
	actJSON, _ := json.Marshal(rule.Actions)
	enabled := 0
	if rule.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_rules SET name=?, trigger=?, conditions=?, actions=?, enabled=?, updated_at=datetime('now')
		 WHERE id=? AND builtin=0`,
		rule.Name, string(rule.Trigger), string(condJSON), string(actJSON), enabled, rule.ID)
	return err
}

func (s *SQLiteRuleStore) DeleteRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM automation_rules WHERE id=? AND builtin=0`, id)
	return err
}

func (s *SQLiteRuleStore) ToggleRule(ctx context.Context, id string, enabled bool) error {
	e := 0
	if enabled {
		e = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_rules SET enabled=?, updated_at=datetime('now') WHERE id=?`, e, id)
	return err
}

func (s *SQLiteRuleStore) RecordExecution(ctx context.Context, entry *HistoryEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO automation_history (id, rule_id, project_id, event_id, status, error, started_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.RuleID, entry.ProjectID, entry.EventID, entry.Status, entry.Error,
		entry.StartedAt.UTC().Format("2006-01-02 15:04:05"),
		entry.EndedAt.UTC().Format("2006-01-02 15:04:05"))
	return err
}

func (s *SQLiteRuleStore) ListHistory(ctx context.Context, projectID string, limit int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, rule_id, project_id, event_id, status, error, started_at, ended_at
		 FROM automation_history WHERE project_id = ? ORDER BY started_at DESC LIMIT ?`,
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
		e.StartedAt, _ = time.Parse("2006-01-02 15:04:05", startedAt)
		e.EndedAt, _ = time.Parse("2006-01-02 15:04:05", endedAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
