package event

import (
	"context"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
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

// HistoryQuery selects one page of a project's automation execution history.
type HistoryQuery struct {
	ProjectID string
	Limit     int
	Cursor    string // opaque; from a previous page's NextCursor
}

// HistoryPage is one page of execution history, newest first. NextCursor is
// empty on the last page.
type HistoryPage struct {
	Entries    []HistoryEntry `json:"entries"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// AutomationHistoryStore persists automation execution history.
type AutomationHistoryStore interface {
	RecordExecution(ctx context.Context, entry *HistoryEntry) error
	ListHistory(ctx context.Context, q HistoryQuery) (*HistoryPage, error)
}
