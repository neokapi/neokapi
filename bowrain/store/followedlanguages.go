package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// FollowedLanguages implements platstore.FollowedLanguagesStore.
func (s *PostgresStore) FollowedLanguages(ctx context.Context, userID, projectID string) ([]string, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT locales FROM followed_languages WHERE user_id = $1 AND project_id = $2`,
		userID, projectID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read the languages %s follows: %w", userID, err)
	}
	return platstore.DecodeLocales(raw), true, nil
}

// PutFollowedLanguages implements platstore.FollowedLanguagesStore.
func (s *PostgresStore) PutFollowedLanguages(ctx context.Context, userID, projectID string, locales []string) error {
	if userID == "" || projectID == "" {
		return errors.New("save followed languages: no user or no project")
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO followed_languages (user_id, project_id, locales, updated_at)
		VALUES ($1, $2, $3::jsonb, NOW())
		ON CONFLICT (user_id, project_id) DO UPDATE
		   SET locales = EXCLUDED.locales, updated_at = EXCLUDED.updated_at`,
		userID, projectID, platstore.EncodeLocales(locales)); err != nil {
		return fmt.Errorf("save the languages %s follows: %w", userID, err)
	}
	return nil
}

// ListFollowedLanguages implements platstore.FollowedLanguagesStore.
func (s *PostgresStore) ListFollowedLanguages(ctx context.Context, userID string) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, locales FROM followed_languages WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("list the languages %s follows: %w", userID, err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var pid string
		var raw []byte
		if err := rows.Scan(&pid, &raw); err != nil {
			return nil, fmt.Errorf("list the languages %s follows: %w", userID, err)
		}
		out[pid] = platstore.DecodeLocales(raw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list the languages %s follows: %w", userID, err)
	}
	return out, nil
}

var _ platstore.FollowedLanguagesStore = (*PostgresStore)(nil)
