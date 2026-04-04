package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// DigestFrequency controls how often digest emails are sent.
type DigestFrequency string

const (
	DigestDaily  DigestFrequency = "daily"
	DigestWeekly DigestFrequency = "weekly"
	DigestOff    DigestFrequency = "off"
)

// DigestSettings holds per-user digest email configuration.
type DigestSettings struct {
	UserID      string          `json:"user_id"`
	WorkspaceID string          `json:"workspace_id"`
	Frequency   DigestFrequency `json:"frequency"`
	QuietStart  string          `json:"quiet_start,omitempty"` // HH:MM format, e.g. "22:00"
	QuietEnd    string          `json:"quiet_end,omitempty"`   // HH:MM format, e.g. "08:00"
	Timezone    string          `json:"timezone,omitempty"`    // IANA timezone, e.g. "America/New_York"
}

// DefaultDigestSettings returns the default digest settings for a user.
func DefaultDigestSettings(userID, workspaceID string) *DigestSettings {
	return &DigestSettings{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Frequency:   DigestDaily,
		Timezone:    "UTC",
	}
}

// DigestState tracks when the last digest was sent for a user.
type DigestState struct {
	UserID      string    `json:"user_id"`
	WorkspaceID string    `json:"workspace_id"`
	Frequency   string    `json:"frequency"`
	LastSentAt  time.Time `json:"last_sent_at"`
}

// DigestStore persists digest settings and state.
type DigestStore struct {
	db      *sql.DB
	dialect Dialect
}

// NewDigestStore creates a SQLite-backed digest store.
func NewDigestStore(db *sql.DB) *DigestStore {
	return &DigestStore{db: db, dialect: DialectSQLite}
}

// NewPostgresDigestStore creates a PostgreSQL-backed digest store.
func NewPostgresDigestStore(db *sql.DB) *DigestStore {
	return &DigestStore{db: db, dialect: DialectPostgres}
}

func (s *DigestStore) q(query string) string {
	return Rebind(s.dialect, query)
}

// GetSettings returns the digest settings for a user in a workspace.
// Returns defaults if not explicitly configured.
func (s *DigestStore) GetSettings(ctx context.Context, userID, workspaceID string) (*DigestSettings, error) {
	var ds DigestSettings
	var freq string
	err := s.db.QueryRowContext(ctx, s.q(
		`SELECT user_id, workspace_id, frequency, quiet_start, quiet_end, timezone
		 FROM digest_settings WHERE user_id = ? AND workspace_id = ?`),
		userID, workspaceID).Scan(&ds.UserID, &ds.WorkspaceID, &freq, &ds.QuietStart, &ds.QuietEnd, &ds.Timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultDigestSettings(userID, workspaceID), nil
	}
	if err != nil {
		return nil, err
	}
	ds.Frequency = DigestFrequency(freq)
	return &ds, nil
}

// UpsertSettings saves digest settings, creating or updating as needed.
func (s *DigestStore) UpsertSettings(ctx context.Context, ds *DigestSettings) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO digest_settings (user_id, workspace_id, frequency, quiet_start, quiet_end, timezone)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (user_id, workspace_id)
		 DO UPDATE SET frequency = ?, quiet_start = ?, quiet_end = ?, timezone = ?`),
		ds.UserID, ds.WorkspaceID, string(ds.Frequency), ds.QuietStart, ds.QuietEnd, ds.Timezone,
		string(ds.Frequency), ds.QuietStart, ds.QuietEnd, ds.Timezone)
	return err
}

// ListUsersWithFrequency returns all user/workspace pairs with a specific digest frequency.
func (s *DigestStore) ListUsersWithFrequency(ctx context.Context, frequency DigestFrequency) ([]DigestSettings, error) {
	rows, err := s.db.QueryContext(ctx, s.q(
		`SELECT user_id, workspace_id, frequency, quiet_start, quiet_end, timezone
		 FROM digest_settings WHERE frequency = ?`), string(frequency))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []DigestSettings
	for rows.Next() {
		var ds DigestSettings
		var freq string
		if err := rows.Scan(&ds.UserID, &ds.WorkspaceID, &freq, &ds.QuietStart, &ds.QuietEnd, &ds.Timezone); err != nil {
			return nil, err
		}
		ds.Frequency = DigestFrequency(freq)
		settings = append(settings, ds)
	}
	return settings, rows.Err()
}

// GetState returns the last digest sent time for a user/workspace/frequency.
func (s *DigestStore) GetState(ctx context.Context, userID, workspaceID, frequency string) (*DigestState, error) {
	var ds DigestState
	var lastSent string
	err := s.db.QueryRowContext(ctx, s.q(
		`SELECT user_id, workspace_id, frequency, last_sent_at
		 FROM digest_state WHERE user_id = ? AND workspace_id = ? AND frequency = ?`),
		userID, workspaceID, frequency).Scan(&ds.UserID, &ds.WorkspaceID, &ds.Frequency, &lastSent)
	if errors.Is(err, sql.ErrNoRows) {
		return &DigestState{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Frequency:   frequency,
			LastSentAt:  time.Now().UTC().Add(-24 * time.Hour), // default: look back 24h
		}, nil
	}
	if err != nil {
		return nil, err
	}
	ds.LastSentAt, _ = parseTime(lastSent)
	return &ds, nil
}

// UpdateState records when a digest was last sent.
func (s *DigestStore) UpdateState(ctx context.Context, userID, workspaceID, frequency string, sentAt time.Time) error {
	_, err := s.db.ExecContext(ctx, s.q(
		`INSERT INTO digest_state (user_id, workspace_id, frequency, last_sent_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (user_id, workspace_id, frequency)
		 DO UPDATE SET last_sent_at = ?`),
		userID, workspaceID, frequency, sentAt.UTC().Format(time.RFC3339),
		sentAt.UTC().Format(time.RFC3339))
	return err
}

// IsInQuietHours checks if the current time falls within the user's quiet hours.
func (s *DigestStore) IsInQuietHours(ds *DigestSettings, now time.Time) bool {
	if ds.QuietStart == "" || ds.QuietEnd == "" {
		return false
	}

	loc, err := time.LoadLocation(ds.Timezone)
	if err != nil {
		return false
	}

	localNow := now.In(loc)
	localHHMM := localNow.Format("15:04")

	// Handle overnight ranges (e.g. 22:00 - 08:00)
	if ds.QuietStart > ds.QuietEnd {
		return localHHMM >= ds.QuietStart || localHHMM < ds.QuietEnd
	}
	return localHHMM >= ds.QuietStart && localHHMM < ds.QuietEnd
}
