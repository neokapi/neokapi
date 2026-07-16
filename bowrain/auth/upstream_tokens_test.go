package auth

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestUpstreamStore returns a migrated store with encryption enabled, plus
// the underlying DB so tests can inspect the raw at-rest column. Skips if Docker
// (testcontainers) is unavailable.
func newTestUpstreamStore(t *testing.T) (*PostgresUpstreamTokenStore, *storage.PgDB) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	key := base64.StdEncoding.EncodeToString(make([]byte, 32)) // 32-byte AES-256 key
	cipher, err := crypto.NewCipher(key)
	require.NoError(t, err)
	require.NotNil(t, cipher)
	s, err := NewUpstreamTokenStoreFromDB(db, cipher)
	require.NoError(t, err)
	return s, db
}

func TestUpstreamTokenStore_PutGetRoundTrip(t *testing.T) {
	s, db := newTestUpstreamStore(t)
	ctx := context.Background()

	err := s.PutRefreshToken(ctx, "user-1", "cognito", "refresh-secret-123", time.Now().Add(time.Hour))
	require.NoError(t, err)

	got, expires, err := s.GetRefreshToken(ctx, "user-1", "cognito")
	require.NoError(t, err)
	assert.Equal(t, "refresh-secret-123", got)
	assert.WithinDuration(t, time.Now().Add(time.Hour), expires, 2*time.Minute)

	// Encrypted at rest: the stored column is sealed, not the plaintext token.
	var raw string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT refresh_enc FROM oidc_upstream_tokens WHERE user_id = $1 AND provider = $2`,
		"user-1", "cognito").Scan(&raw))
	assert.True(t, strings.HasPrefix(raw, "enc:v1:"), "token should be sealed at rest, got %q", raw)
	assert.NotContains(t, raw, "refresh-secret-123")
}

func TestUpstreamTokenStore_Upsert(t *testing.T) {
	s, _ := newTestUpstreamStore(t)
	ctx := context.Background()

	require.NoError(t, s.PutRefreshToken(ctx, "user-1", "cognito", "first", time.Now().Add(time.Hour)))
	require.NoError(t, s.PutRefreshToken(ctx, "user-1", "cognito", "second", time.Now().Add(time.Hour)))

	got, _, err := s.GetRefreshToken(ctx, "user-1", "cognito")
	require.NoError(t, err)
	assert.Equal(t, "second", got)
}

func TestUpstreamTokenStore_NotFound(t *testing.T) {
	s, _ := newTestUpstreamStore(t)
	_, _, err := s.GetRefreshToken(context.Background(), "nobody", "cognito")
	assert.ErrorIs(t, err, ErrUpstreamTokenNotFound)
}

func TestUpstreamTokenStore_ExpiredTreatedAsGone(t *testing.T) {
	s, db := newTestUpstreamStore(t)
	ctx := context.Background()

	require.NoError(t, s.PutRefreshToken(ctx, "user-1", "cognito", "stale", time.Now().Add(-time.Minute)))
	_, _, err := s.GetRefreshToken(ctx, "user-1", "cognito")
	assert.ErrorIs(t, err, ErrUpstreamTokenNotFound)

	// Expired rows are pruned on read.
	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM oidc_upstream_tokens WHERE user_id = $1`, "user-1").Scan(&count))
	assert.Zero(t, count)
}

func TestUpstreamTokenStore_Delete(t *testing.T) {
	s, _ := newTestUpstreamStore(t)
	ctx := context.Background()

	require.NoError(t, s.PutRefreshToken(ctx, "user-1", "cognito", "tok", time.Now().Add(time.Hour)))
	require.NoError(t, s.DeleteRefreshToken(ctx, "user-1", "cognito"))
	_, _, err := s.GetRefreshToken(ctx, "user-1", "cognito")
	assert.ErrorIs(t, err, ErrUpstreamTokenNotFound)

	// Deleting a missing row is a no-op.
	require.NoError(t, s.DeleteRefreshToken(ctx, "user-1", "cognito"))
}

func TestUpstreamTokenStore_Validation(t *testing.T) {
	s, _ := newTestUpstreamStore(t)
	ctx := context.Background()
	assert.Error(t, s.PutRefreshToken(ctx, "", "cognito", "tok", time.Now().Add(time.Hour)))
	assert.Error(t, s.PutRefreshToken(ctx, "user", "", "tok", time.Now().Add(time.Hour)))
	assert.Error(t, s.PutRefreshToken(ctx, "user", "cognito", "", time.Now().Add(time.Hour)))
}
