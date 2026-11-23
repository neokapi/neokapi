package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/contextop"
)

const contextSessionColumns = `id, project_id, user_id, cards, reach, decisions, started_at, expires_at, ended_at`

// StartContextSession implements platstore.ContextSessionStore. Each step is
// its own statement, so two sessions started at once never wait on each
// other's locks: the hold key settles which of them takes a card.
func (s *PostgresStore) StartContextSession(ctx context.Context, sess platstore.ContextSession, items []contextop.ItemRef, limit int) ([]contextop.ItemRef, error) {
	if sess.ID == "" || sess.ProjectID == "" || sess.UserID == "" {
		return nil, errors.New("start a context session: no id, project or user")
	}
	// One transaction: a session whose holds cannot be written does not
	// start, so no session ever exists without the cards it holds.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("start a context session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := sess.StartedAt.UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE context_sessions SET ended_at = expires_at
		 WHERE project_id = $1 AND ended_at IS NULL AND expires_at <= $2`, sess.ProjectID, now); err != nil {
		return nil, fmt.Errorf("end expired context sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM context_session_holds
		 WHERE project_id = $1
		   AND session_id IN (SELECT id FROM context_sessions WHERE project_id = $1 AND ended_at IS NOT NULL)`,
		sess.ProjectID); err != nil {
		return nil, fmt.Errorf("release ended context sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO context_sessions (id, project_id, user_id, cards, reach, decisions, started_at, expires_at)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, '[]'::jsonb, $6, $7)`,
		sess.ID, sess.ProjectID, sess.UserID,
		platstore.EncodeSessionJSON(storableCards(sess.Cards), "[]"), platstore.EncodeSessionJSON(platstore.StorableReach(sess.Reach), "{}"),
		now, sess.ExpiresAt.UTC()); err != nil {
		return nil, fmt.Errorf("start a context session: %w", err)
	}
	var held []contextop.ItemRef
	for _, item := range items {
		if len(held) >= limit {
			break
		}
		if !platstore.StorableItem(item) {
			continue // a card the session cannot name is not dealt
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO context_session_holds (project_id, item_kind, item_profile, item_id, session_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (project_id, item_kind, item_profile, item_id) DO NOTHING`,
			sess.ProjectID, string(item.Kind), item.Profile, item.ID, sess.ID)
		if err != nil {
			return nil, fmt.Errorf("hold a context card: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 1 {
			held = append(held, item)
		}
	}
	if sess.DealCards != nil {
		dealt, err := sess.DealCards(held)
		if err != nil {
			return nil, fmt.Errorf("deal the cards of a context session: %w", err)
		}
		cards := storableCards(dealt)
		if _, err := tx.ExecContext(ctx, `UPDATE context_sessions SET cards = $2::jsonb WHERE id = $1`,
			sess.ID, platstore.EncodeSessionJSON(cards, "[]")); err != nil {
			return nil, fmt.Errorf("record the cards of context session %s: %w", sess.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("start a context session: %w", err)
	}
	return held, nil
}

// storableCards is platstore.StorableCards without the count.
func storableCards(cards []platstore.ContextSessionCard) []platstore.ContextSessionCard {
	out, _ := platstore.StorableCards(cards)
	return out
}

// SetContextSessionCards implements platstore.ContextSessionStore.
func (s *PostgresStore) SetContextSessionCards(ctx context.Context, id string, cards []platstore.ContextSessionCard) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE context_sessions SET cards = $2::jsonb WHERE id = $1`,
		id, platstore.EncodeSessionJSON(storableCards(cards), "[]")); err != nil {
		return fmt.Errorf("record the cards of context session %s: %w", id, err)
	}
	return nil
}

// GetContextSession implements platstore.ContextSessionStore.
func (s *PostgresStore) GetContextSession(ctx context.Context, id string) (platstore.ContextSession, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+contextSessionColumns+` FROM context_sessions WHERE id = $1`, id)
	sess, err := scanPgContextSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return platstore.ContextSession{}, false, nil
	}
	if err != nil {
		return platstore.ContextSession{}, false, fmt.Errorf("read context session %s: %w", id, err)
	}
	return sess, true, nil
}

// ContextSessionHolds implements platstore.ContextSessionStore.
func (s *PostgresStore) ContextSessionHolds(ctx context.Context, projectID string, now time.Time) ([]platstore.ContextSessionHold, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT h.item_kind, h.item_profile, h.item_id, h.session_id, cs.user_id
		  FROM context_session_holds h JOIN context_sessions cs ON cs.id = h.session_id
		 WHERE h.project_id = $1 AND cs.ended_at IS NULL AND cs.expires_at > $2
		 ORDER BY h.item_kind, h.item_profile, h.item_id`, projectID, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("list the held context cards of project %s: %w", projectID, err)
	}
	defer rows.Close()
	var out []platstore.ContextSessionHold
	for rows.Next() {
		var h platstore.ContextSessionHold
		var kind string
		if err := rows.Scan(&kind, &h.Item.Profile, &h.Item.ID, &h.SessionID, &h.UserID); err != nil {
			return nil, fmt.Errorf("list the held context cards of project %s: %w", projectID, err)
		}
		h.Item.Kind = contextop.ItemKind(kind)
		out = append(out, h)
	}
	return out, rows.Err()
}

// RecordContextSessionDecision implements platstore.ContextSessionStore.
func (s *PostgresStore) RecordContextSessionDecision(ctx context.Context, id string, d platstore.ContextSessionDecision, expires time.Time) error {
	body, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE context_sessions
		   SET decisions = decisions || jsonb_build_array($2::jsonb), last_decided_at = $3, expires_at = $4
		 WHERE id = $1`, id, string(body), d.At.UTC(), expires.UTC()); err != nil {
		return fmt.Errorf("record a decision of context session %s: %w", id, err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM context_session_holds
		  WHERE session_id = $1 AND item_kind = $2 AND item_profile = $3 AND item_id = $4`,
		id, string(d.Item.Kind), d.Item.Profile, d.Item.ID); err != nil {
		return fmt.Errorf("release a card of context session %s: %w", id, err)
	}
	return nil
}

// EndContextSession implements platstore.ContextSessionStore.
func (s *PostgresStore) EndContextSession(ctx context.Context, id string, at time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE context_sessions SET ended_at = LEAST($2, expires_at)
		 WHERE id = $1 AND ended_at IS NULL`, id, at.UTC()); err != nil {
		return fmt.Errorf("end context session %s: %w", id, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM context_session_holds WHERE session_id = $1`, id); err != nil {
		return fmt.Errorf("release the cards of context session %s: %w", id, err)
	}
	return nil
}

// LastEndedContextSession implements platstore.ContextSessionStore.
func (s *PostgresStore) LastEndedContextSession(ctx context.Context, projectID string, now time.Time) (platstore.ContextSession, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+contextSessionColumns+` FROM context_sessions
		 WHERE project_id = $1 AND (ended_at IS NOT NULL OR expires_at <= $2)
		 ORDER BY COALESCE(ended_at, expires_at) DESC, id DESC LIMIT 1`, projectID, now.UTC())
	sess, err := scanPgContextSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return platstore.ContextSession{}, false, nil
	}
	if err != nil {
		return platstore.ContextSession{}, false, fmt.Errorf("read the last context session of project %s: %w", projectID, err)
	}
	if sess.EndedAt.IsZero() {
		sess.EndedAt = sess.ExpiresAt
	}
	return sess, true, nil
}

// ContextSessionsSince implements platstore.ContextSessionStore.
func (s *PostgresStore) ContextSessionsSince(ctx context.Context, projectID string, since time.Time) ([]platstore.ContextSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+contextSessionColumns+` FROM context_sessions
		 WHERE project_id = $1 AND last_decided_at > $2
		 ORDER BY started_at, id`, projectID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("list the context sessions of project %s: %w", projectID, err)
	}
	defer rows.Close()
	var out []platstore.ContextSession
	for rows.Next() {
		sess, err := scanPgContextSession(rows)
		if err != nil {
			return nil, fmt.Errorf("list the context sessions of project %s: %w", projectID, err)
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// scanPgContextSession reads one row of contextSessionColumns. An expired
// session reads as ended at its expiry.
func scanPgContextSession(row interface{ Scan(dest ...any) error }) (platstore.ContextSession, error) {
	var sess platstore.ContextSession
	var cards, reach, decisions []byte
	var ended sql.NullTime
	if err := row.Scan(&sess.ID, &sess.ProjectID, &sess.UserID, &cards, &reach, &decisions,
		&sess.StartedAt, &sess.ExpiresAt, &ended); err != nil {
		return sess, err
	}
	sess.StartedAt, sess.ExpiresAt = sess.StartedAt.UTC(), sess.ExpiresAt.UTC()
	if ended.Valid {
		sess.EndedAt = ended.Time.UTC()
	}
	if err := decodeSessionFields(&sess, cards, reach, decisions); err != nil {
		return sess, err
	}
	return sess, nil
}

// decodeSessionFields reads a session's JSON columns.
func decodeSessionFields(sess *platstore.ContextSession, cards, reach, decisions []byte) error {
	if err := json.Unmarshal(cards, &sess.Cards); err != nil {
		return fmt.Errorf("decode session cards: %w", err)
	}
	if err := json.Unmarshal(reach, &sess.Reach); err != nil {
		return fmt.Errorf("decode session reach: %w", err)
	}
	if err := json.Unmarshal(decisions, &sess.Decisions); err != nil {
		return fmt.Errorf("decode session decisions: %w", err)
	}
	for i := range sess.Decisions {
		sess.Decisions[i].At = sess.Decisions[i].At.UTC()
	}
	return nil
}

var _ platstore.ContextSessionStore = (*PostgresStore)(nil)
