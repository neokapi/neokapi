package event

import (
	"context"
	"time"

	platev "github.com/neokapi/neokapi/platform/event"
)

// StoredRule represents a persisted automation rule.
type StoredRule struct {
	ID         string                `json:"id"`
	ProjectID  string                `json:"project_id"`
	Name       string                `json:"name"`
	Trigger    platev.EventType      `json:"trigger"`
	Conditions []AutomationCondition `json:"conditions"`
	Actions    []AutomationAction    `json:"actions"`
	Enabled    bool                  `json:"enabled"`
	Builtin    bool                  `json:"builtin"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

// HistoryEntry represents a single automation execution record.
type HistoryEntry struct {
	ID        string    `json:"id"`
	RuleID    string    `json:"rule_id"`
	ProjectID string    `json:"project_id"`
	EventID   string    `json:"event_id"`
	Status    string    `json:"status"` // "success", "failed", "skipped"
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

// AutomationRuleStore persists automation rules.
type AutomationRuleStore interface {
	ListRules(ctx context.Context, projectID string) ([]StoredRule, error)
	GetRule(ctx context.Context, id string) (*StoredRule, error)
	CreateRule(ctx context.Context, rule *StoredRule) error
	UpdateRule(ctx context.Context, rule *StoredRule) error
	DeleteRule(ctx context.Context, id string) error
	ToggleRule(ctx context.Context, id string, enabled bool) error
}

// AutomationHistoryStore persists automation execution history.
type AutomationHistoryStore interface {
	RecordExecution(ctx context.Context, entry *HistoryEntry) error
	ListHistory(ctx context.Context, projectID string, limit int) ([]HistoryEntry, error)
}
