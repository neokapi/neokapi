package sqlitestore

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

// sessionTime is the layout session times are stored in: fixed width, so
// the strings sort as the times do.
const sessionTime = "2006-01-02T15:04:05.000000000Z"

func sessionStamp(t time.Time) string { return t.UTC().Format(sessionTime) }

const contextSessionColumns = `id, project_id, user_id, cards, reach, decisions, started_at, expires_at, ended_at`

// StartContextSession implements platstore.ContextSessionStore.
func (s *SQLiteStore) StartContextSession(ctx context.Context, sess platstore.ContextSession, items []contextop.ItemRef, limit int) ([]contextop.ItemRef, error) {
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
	now := sessionStamp(sess.StartedAt)
	if _, err := tx.ExecContext(ctx, `
		UPDATE context_sessions SET ended_at = expires_at
		 WHERE project_id = ? AND ended_at = '' AND expires_at <= ?`, sess.ProjectID, now); err != nil {
		return nil, fmt.Errorf("end expired context sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM context_session_holds
		 WHERE project_id = ?
		   AND session_id IN (SELECT id FROM context_sessions WHERE project_id = ? AND ended_at <> '')`,
		sess.ProjectID, sess.ProjectID); err != nil {
		return nil, fmt.Errorf("release ended context sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO context_sessions (id, project_id, user_id, cards, reach, decisions, started_at, expires_at)
		VALUES (?, ?, ?, ?, ?, '[]', ?, ?)`,
		sess.ID, sess.ProjectID, sess.UserID,
		platstore.EncodeSessionJSON(storableCards(sess.Cards), "[]"), platstore.EncodeSessionJSON(platstore.StorableReach(sess.Reach), "{}"),
		now, sessionStamp(sess.ExpiresAt)); err != nil {
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
			VALUES (?, ?, ?, ?, ?)
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
		if _, err := tx.ExecContext(ctx, `UPDATE context_sessions SET cards = ? WHERE id = ?`,
			platstore.EncodeSessionJSON(cards, "[]"), sess.ID); err != nil {
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
func (s *SQLiteStore) SetContextSessionCards(ctx context.Context, id string, cards []platstore.ContextSessionCard) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE context_sessions SET cards = ? WHERE id = ?`,
		platstore.EncodeSessionJSON(storableCards(cards), "[]"), id); err != nil {
		return fmt.Errorf("record the cards of context session %s: %w", id, err)
	}
	return nil
}

// GetContextSession implements platstore.ContextSessionStore.
func (s *SQLiteStore) GetContextSession(ctx context.Context, id string) (platstore.ContextSession, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+contextSessionColumns+` FROM context_sessions WHERE id = ?`, id)
	sess, err := scanSQLiteContextSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return platstore.ContextSession{}, false, nil
	}
	if err != nil {
		return platstore.ContextSession{}, false, fmt.Errorf("read context session %s: %w", id, err)
	}
	return sess, true, nil
}

// ContextSessionHolds implements platstore.ContextSessionStore.
func (s *SQLiteStore) ContextSessionHolds(ctx context.Context, projectID string, now time.Time) ([]platstore.ContextSessionHold, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT h.item_kind, h.item_profile, h.item_id, h.session_id, cs.user_id
		  FROM context_session_holds h JOIN context_sessions cs ON cs.id = h.session_id
		 WHERE h.project_id = ? AND cs.ended_at = '' AND cs.expires_at > ?
		 ORDER BY h.item_kind, h.item_profile, h.item_id`, projectID, sessionStamp(now))
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
func (s *SQLiteStore) RecordContextSessionDecision(ctx context.Context, id string, d platstore.ContextSessionDecision, expires time.Time) error {
	d.At = d.At.UTC()
	body, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE context_sessions
		   SET decisions = json_insert(decisions, '$[#]', json(?)), last_decided_at = ?, expires_at = ?
		 WHERE id = ?`, string(body), sessionStamp(d.At), sessionStamp(expires), id); err != nil {
		return fmt.Errorf("record a decision of context session %s: %w", id, err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM context_session_holds
		  WHERE session_id = ? AND item_kind = ? AND item_profile = ? AND item_id = ?`,
		id, string(d.Item.Kind), d.Item.Profile, d.Item.ID); err != nil {
		return fmt.Errorf("release a card of context session %s: %w", id, err)
	}
	return nil
}

// EndContextSession implements platstore.ContextSessionStore.
func (s *SQLiteStore) EndContextSession(ctx context.Context, id string, at time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE context_sessions SET ended_at = MIN(?, expires_at)
		 WHERE id = ? AND ended_at = ''`, sessionStamp(at), id); err != nil {
		return fmt.Errorf("end context session %s: %w", id, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM context_session_holds WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("release the cards of context session %s: %w", id, err)
	}
	return nil
}

// LastEndedContextSession implements platstore.ContextSessionStore.
func (s *SQLiteStore) LastEndedContextSession(ctx context.Context, projectID string, now time.Time) (platstore.ContextSession, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+contextSessionColumns+` FROM context_sessions
		 WHERE project_id = ? AND (ended_at <> '' OR expires_at <= ?)
		 ORDER BY CASE WHEN ended_at <> '' THEN ended_at ELSE expires_at END DESC, id DESC LIMIT 1`,
		projectID, sessionStamp(now))
	sess, err := scanSQLiteContextSession(row)
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
func (s *SQLiteStore) ContextSessionsSince(ctx context.Context, projectID string, since time.Time) ([]platstore.ContextSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+contextSessionColumns+` FROM context_sessions
		 WHERE project_id = ? AND last_decided_at <> '' AND last_decided_at > ?
		 ORDER BY started_at, id`, projectID, sessionStamp(since))
	if err != nil {
		return nil, fmt.Errorf("list the context sessions of project %s: %w", projectID, err)
	}
	defer rows.Close()
	var out []platstore.ContextSession
	for rows.Next() {
		sess, err := scanSQLiteContextSession(rows)
		if err != nil {
			return nil, fmt.Errorf("list the context sessions of project %s: %w", projectID, err)
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// scanSQLiteContextSession reads one row of contextSessionColumns.
func scanSQLiteContextSession(row interface{ Scan(dest ...any) error }) (platstore.ContextSession, error) {
	var sess platstore.ContextSession
	var cards, reach, decisions, started, expires, ended string
	if err := row.Scan(&sess.ID, &sess.ProjectID, &sess.UserID, &cards, &reach, &decisions,
		&started, &expires, &ended); err != nil {
		return sess, err
	}
	var err error
	if sess.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
		return sess, err
	}
	if sess.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires); err != nil {
		return sess, err
	}
	if ended != "" {
		if sess.EndedAt, err = time.Parse(time.RFC3339Nano, ended); err != nil {
			return sess, err
		}
	}
	if err := json.Unmarshal([]byte(cards), &sess.Cards); err != nil {
		return sess, fmt.Errorf("decode session cards: %w", err)
	}
	if err := json.Unmarshal([]byte(reach), &sess.Reach); err != nil {
		return sess, fmt.Errorf("decode session reach: %w", err)
	}
	if err := json.Unmarshal([]byte(decisions), &sess.Decisions); err != nil {
		return sess, fmt.Errorf("decode session decisions: %w", err)
	}
	for i := range sess.Decisions {
		sess.Decisions[i].At = sess.Decisions[i].At.UTC()
	}
	return sess, nil
}

var _ platstore.ContextSessionStore = (*SQLiteStore)(nil)
