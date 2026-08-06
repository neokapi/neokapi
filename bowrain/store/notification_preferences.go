package store

import (
	"context"
	"database/sql"
	"errors"
)

// NotificationCategory groups notification types for preference management.
type NotificationCategory string

const (
	CategoryTask       NotificationCategory = "task"
	CategoryReview     NotificationCategory = "review"
	CategoryQuality    NotificationCategory = "quality"
	CategoryAutomation NotificationCategory = "automation"
	CategoryMention    NotificationCategory = "mention"
	CategoryProject    NotificationCategory = "project"
	CategorySystem     NotificationCategory = "system"
)

// NotificationPreference defines channel settings for a notification category.
type NotificationPreference struct {
	UserID      string               `json:"user_id"`
	WorkspaceID string               `json:"workspace_id"`
	Category    NotificationCategory `json:"category"`
	Web         bool                 `json:"channel_web"`
	Email       bool                 `json:"channel_email"`
	Push        bool                 `json:"channel_push"`
	Desktop     bool                 `json:"channel_desktop"`
}

// DefaultPreferences returns the default notification preferences for all categories.
func DefaultPreferences(userID, workspaceID string) []NotificationPreference {
	return []NotificationPreference{
		{userID, workspaceID, CategoryTask, true, true, true, true},
		{userID, workspaceID, CategoryReview, true, false, false, true},
		{userID, workspaceID, CategoryQuality, true, false, false, true},
		{userID, workspaceID, CategoryAutomation, true, false, false, false},
		{userID, workspaceID, CategoryMention, true, true, true, true},
		{userID, workspaceID, CategoryProject, true, false, false, false},
		{userID, workspaceID, CategorySystem, true, true, false, false},
	}
}

// PreferenceStore persists notification preferences.
type PreferenceStore struct {
	db *sql.DB
}

// NewPreferenceStore creates a PostgreSQL-backed preference store.
func NewPreferenceStore(db *sql.DB) *PreferenceStore {
	return &PreferenceStore{db: db}
}

// List returns all notification preferences for a user in a workspace.
// Returns defaults for categories not explicitly configured.
func (s *PreferenceStore) List(ctx context.Context, userID, workspaceID string) ([]NotificationPreference, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, workspace_id, category, channel_web, channel_email, channel_push, channel_desktop
		 FROM notification_preferences
		 WHERE user_id = $1 AND workspace_id = $2`,
		userID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stored := make(map[NotificationCategory]NotificationPreference)
	for rows.Next() {
		var p NotificationPreference
		var cat string
		if err := rows.Scan(&p.UserID, &p.WorkspaceID, &cat, &p.Web, &p.Email, &p.Push, &p.Desktop); err != nil {
			return nil, err
		}
		p.Category = NotificationCategory(cat)
		stored[p.Category] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Merge with defaults.
	defaults := DefaultPreferences(userID, workspaceID)
	result := make([]NotificationPreference, 0, len(defaults))
	for _, d := range defaults {
		if s, ok := stored[d.Category]; ok {
			result = append(result, s)
		} else {
			result = append(result, d)
		}
	}

	return result, nil
}

// Get returns the preference for a specific category. Returns the default if not explicitly set.
func (s *PreferenceStore) Get(ctx context.Context, userID, workspaceID string, category NotificationCategory) (*NotificationPreference, error) {
	prefs, err := s.List(ctx, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, p := range prefs {
		if p.Category == category {
			return &p, nil
		}
	}
	// Return a default.
	for _, d := range DefaultPreferences(userID, workspaceID) {
		if d.Category == category {
			return &d, nil
		}
	}
	return &NotificationPreference{UserID: userID, WorkspaceID: workspaceID, Category: category, Web: true, Desktop: true}, nil
}

// Lookup returns the preference the user actually stored for a category, and
// whether they stored one at all.
//
// Get and List cannot answer that: both fold DefaultPreferences in, so a user
// who has never opened notification preferences is indistinguishable from one
// who deliberately turned a channel off. That distinction is the whole of
// "on by default, off if you say so" — a caller whose default differs from the
// category's (a job failure mails, while the rest of the automation category
// does not) needs to honour a stored "no" without inheriting a default "no".
func (s *PreferenceStore) Lookup(ctx context.Context, userID, workspaceID string, category NotificationCategory) (*NotificationPreference, bool, error) {
	var p NotificationPreference
	var cat string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, workspace_id, category, channel_web, channel_email, channel_push, channel_desktop
		 FROM notification_preferences
		 WHERE user_id = $1 AND workspace_id = $2 AND category = $3`,
		userID, workspaceID, string(category)).Scan(
		&p.UserID, &p.WorkspaceID, &cat, &p.Web, &p.Email, &p.Push, &p.Desktop)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	p.Category = NotificationCategory(cat)
	return &p, true, nil
}

// Upsert saves a notification preference, creating or updating as needed.
func (s *PreferenceStore) Upsert(ctx context.Context, p *NotificationPreference) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notification_preferences (user_id, workspace_id, category, channel_web, channel_email, channel_push, channel_desktop)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (user_id, workspace_id, category)
		 DO UPDATE SET channel_web = $4, channel_email = $5, channel_push = $6, channel_desktop = $7`,
		p.UserID, p.WorkspaceID, string(p.Category),
		p.Web, p.Email, p.Push, p.Desktop)
	return err
}

// BulkUpsert saves multiple preferences in a transaction.
func (s *PreferenceStore) BulkUpsert(ctx context.Context, prefs []NotificationPreference) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, p := range prefs {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO notification_preferences (user_id, workspace_id, category, channel_web, channel_email, channel_push, channel_desktop)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (user_id, workspace_id, category)
			 DO UPDATE SET channel_web = $4, channel_email = $5, channel_push = $6, channel_desktop = $7`,
			p.UserID, p.WorkspaceID, string(p.Category),
			p.Web, p.Email, p.Push, p.Desktop)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
