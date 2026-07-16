package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/neokapi/neokapi/bowrain/storage"
)

// ErrUpstreamTokenNotFound is returned by GetRefreshToken when no live token is
// stored for the (user, provider) pair — either it was never retained or it has
// expired. Callers treat this as "the user must sign in again before the
// operation can proceed" (a 409 reauth_required at the HTTP layer).
var ErrUpstreamTokenNotFound = errors.New("upstream refresh token not found or expired")

// UpstreamTokenStore persists a user's upstream identity-provider refresh token
// (encrypted at rest) so the BFF can mint a fresh user-scoped access token on
// demand for self-service operations (passkey management) — without ever
// keeping an IdP token in the browser.
//
// The BFF discards the upstream access/refresh tokens at login (it mints its
// own session); this store re-introduces just the refresh token, sealed under
// the server secrets key, keyed by (user_id, provider). It is deliberately the
// minimal slice of a full upstream-token store: one provider refresh token per
// user, read only server-side when a user-scoped IdP call is needed.
type UpstreamTokenStore interface {
	// PutRefreshToken stores (or replaces) the encrypted refresh token for the
	// user+provider, with an absolute expiry after which it is treated as gone.
	PutRefreshToken(ctx context.Context, userID, provider, refreshToken string, expiresAt time.Time) error
	// GetRefreshToken returns the decrypted refresh token and its expiry, or
	// ErrUpstreamTokenNotFound if absent or expired.
	GetRefreshToken(ctx context.Context, userID, provider string) (refreshToken string, expiresAt time.Time, err error)
	// DeleteRefreshToken removes any stored token for user+provider. It is a
	// no-op (nil) when nothing is stored.
	DeleteRefreshToken(ctx context.Context, userID, provider string) error
	Close() error
}

// upstreamTokenMigrationsPg is the PostgreSQL schema for the upstream-token
// store. Namespaced (oidc_upstream_tokens_schema_migrations) so it versions
// independently of the auth schema that shares the database. Append-only.
var upstreamTokenMigrationsPg = []storage.Migration{
	{
		Version:     1,
		Description: "oidc_upstream_tokens (encrypted per-user, per-provider IdP refresh token)",
		SQL: `CREATE TABLE IF NOT EXISTS oidc_upstream_tokens (
			user_id     TEXT NOT NULL,
			provider    TEXT NOT NULL,
			refresh_enc TEXT NOT NULL,
			expires_at  TIMESTAMPTZ NOT NULL,
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, provider)
		);`,
	},
}

// PostgresUpstreamTokenStore implements UpstreamTokenStore over PostgreSQL. The
// refresh token is sealed with the shared server cipher (AES-256-GCM under
// BOWRAIN_SECRETS_KEY) before it touches the database; a nil cipher degrades to
// the same pass-through the rest of the platform uses (plaintext at rest, with
// a startup warning elsewhere), so an unconfigured key does not break the flow.
type PostgresUpstreamTokenStore struct {
	db     *storage.PgDB
	cipher *crypto.Cipher
}

// NewUpstreamTokenStoreFromDB wraps an existing PgDB and runs the store's
// migration. The cipher may be nil (encryption disabled / pass-through).
func NewUpstreamTokenStoreFromDB(db *storage.PgDB, cipher *crypto.Cipher) (*PostgresUpstreamTokenStore, error) {
	if err := storage.MigratePostgresNS(db, "oidc_upstream_tokens_schema_migrations", upstreamTokenMigrationsPg); err != nil {
		return nil, fmt.Errorf("migrate oidc_upstream_tokens schema: %w", err)
	}
	return &PostgresUpstreamTokenStore{db: db, cipher: cipher}, nil
}

func (s *PostgresUpstreamTokenStore) PutRefreshToken(ctx context.Context, userID, provider, refreshToken string, expiresAt time.Time) error {
	if userID == "" || provider == "" {
		return errors.New("user id and provider are required")
	}
	if refreshToken == "" {
		return errors.New("refresh token is required")
	}
	sealed, err := s.cipher.Seal(refreshToken)
	if err != nil {
		return fmt.Errorf("seal refresh token: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO oidc_upstream_tokens (user_id, provider, refresh_enc, expires_at, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (user_id, provider)
		 DO UPDATE SET refresh_enc = EXCLUDED.refresh_enc, expires_at = EXCLUDED.expires_at, updated_at = now()`,
		userID, provider, sealed, expiresAt.UTC())
	if err != nil {
		return fmt.Errorf("upsert upstream token: %w", err)
	}
	return nil
}

func (s *PostgresUpstreamTokenStore) GetRefreshToken(ctx context.Context, userID, provider string) (string, time.Time, error) {
	var sealed string
	var expiresAt time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT refresh_enc, expires_at FROM oidc_upstream_tokens WHERE user_id = $1 AND provider = $2`,
		userID, provider).Scan(&sealed, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, ErrUpstreamTokenNotFound
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("query upstream token: %w", err)
	}
	if time.Now().After(expiresAt) {
		// Expired: prune best-effort and report as gone so the caller forces a
		// fresh login rather than attempting a doomed refresh.
		_ = s.DeleteRefreshToken(ctx, userID, provider)
		return "", expiresAt, ErrUpstreamTokenNotFound
	}
	plain, err := s.cipher.Open(sealed)
	if err != nil {
		return "", expiresAt, fmt.Errorf("open refresh token: %w", err)
	}
	return plain, expiresAt, nil
}

func (s *PostgresUpstreamTokenStore) DeleteRefreshToken(ctx context.Context, userID, provider string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM oidc_upstream_tokens WHERE user_id = $1 AND provider = $2`, userID, provider)
	if err != nil {
		return fmt.Errorf("delete upstream token: %w", err)
	}
	return nil
}

// Close is a no-op: the store shares the server's PgDB pool, whose lifecycle is
// owned by the auth store / server. Present to satisfy UpstreamTokenStore.
func (s *PostgresUpstreamTokenStore) Close() error { return nil }

// Compile-time assertion that the Postgres store satisfies the port.
var _ UpstreamTokenStore = (*PostgresUpstreamTokenStore)(nil)
