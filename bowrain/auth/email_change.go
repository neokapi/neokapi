package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/id"
)

// EmailChangeTokenTTL is the default validity window for an email-change
// verification token. Confirmation links shorter than this are honored;
// expired requests are purged by PurgeExpiredEmailChangeRequests.
const EmailChangeTokenTTL = 24 * time.Hour

// CreateEmailChangeRequest persists a pending email-change request. The
// caller hashes the plaintext token; only the hash is stored.
func (s *PostgresAuthStore) CreateEmailChangeRequest(ctx context.Context, req *platauth.EmailChangeRequest, tokenHash string) error {
	if req.UserID == "" {
		return errors.New("user ID is required")
	}
	if req.NewEmail == "" {
		return errors.New("new email is required")
	}
	if tokenHash == "" {
		return errors.New("token hash is required")
	}
	if req.ID == "" {
		req.ID = id.New()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	if req.ExpiresAt.IsZero() {
		req.ExpiresAt = req.CreatedAt.Add(EmailChangeTokenTTL)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO email_change_requests (id, user_id, new_email, token_hash, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		req.ID, req.UserID, req.NewEmail, tokenHash, req.ExpiresAt, req.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert email change request: %w", err)
	}
	return nil
}

// GetEmailChangeRequestByToken returns the request matching the token hash,
// or an error if not found. Expired requests are returned as found; callers
// must check ExpiresAt and reject if past.
func (s *PostgresAuthStore) GetEmailChangeRequestByToken(ctx context.Context, tokenHash string) (*platauth.EmailChangeRequest, error) {
	var req platauth.EmailChangeRequest
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, new_email, expires_at, created_at
		 FROM email_change_requests WHERE token_hash = $1`, tokenHash).
		Scan(&req.ID, &req.UserID, &req.NewEmail, &req.ExpiresAt, &req.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get email change request: %w", err)
	}
	return &req, nil
}

// DeleteEmailChangeRequestsForUser removes all pending requests for a user,
// used after a successful confirmation or when the user cancels.
func (s *PostgresAuthStore) DeleteEmailChangeRequestsForUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM email_change_requests WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete email change requests: %w", err)
	}
	return nil
}

// PurgeExpiredEmailChangeRequests removes requests whose tokens are past TTL.
func (s *PostgresAuthStore) PurgeExpiredEmailChangeRequests(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM email_change_requests WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("purge email change requests: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
