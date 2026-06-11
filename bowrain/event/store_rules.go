package event

import (
	"context"
	"database/sql"
	"encoding/json"
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

func (s *RuleStore) ListHistory(ctx context.Context, projectID string, limit int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, rule_id, project_id, event_id, status, error, started_at, ended_at
		 FROM automation_history WHERE project_id = $1 ORDER BY started_at DESC LIMIT $2`,
		projectID, limit)
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
	return entries, rows.Err()
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
	_ = json.Unmarshal([]byte(condJSON), &r.Conditions)
	_ = json.Unmarshal([]byte(actJSON), &r.Actions)
	return &r, nil
}
