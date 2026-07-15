package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// TestRotateRefreshToken exercises rotating-refresh-token semantics: a
// successful rotation consumes the presented token and mints a successor in the
// same family; replaying a consumed token is detected as reuse and revokes the
// whole family; unknown and expired tokens are rejected as invalid.
func TestRotateRefreshToken(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := &platauth.User{ID: "u-rot-1", Email: "rot@example.com", Name: "Rot"}
	require.NoError(t, s.CreateUser(ctx, user))

	future := time.Now().Add(30 * 24 * time.Hour)

	t.Run("happy path rotates and chains in one family", func(t *testing.T) {
		h0 := hashToken("tok-0")
		_, err := s.StoreRefreshToken(ctx, user.ID, h0, future)
		require.NoError(t, err)

		h1 := hashToken("tok-1")
		uid, err := s.RotateRefreshToken(ctx, h0, h1, future)
		require.NoError(t, err)
		assert.Equal(t, user.ID, uid)

		// The successor is itself rotatable.
		h2 := hashToken("tok-2")
		uid, err = s.RotateRefreshToken(ctx, h1, h2, future)
		require.NoError(t, err)
		assert.Equal(t, user.ID, uid)
	})

	t.Run("reuse of a consumed token revokes the family", func(t *testing.T) {
		hA := hashToken("chain-A")
		_, err := s.StoreRefreshToken(ctx, user.ID, hA, future)
		require.NoError(t, err)

		hB := hashToken("chain-B")
		_, err = s.RotateRefreshToken(ctx, hA, hB, future) // hA consumed, hB active
		require.NoError(t, err)

		// Replay the already-consumed hA → reuse detected.
		_, err = s.RotateRefreshToken(ctx, hA, hashToken("chain-X"), future)
		require.ErrorIs(t, err, ErrRefreshTokenReuse)

		// Family revoked: the previously-active successor hB is now invalid too.
		_, err = s.RotateRefreshToken(ctx, hB, hashToken("chain-Y"), future)
		require.ErrorIs(t, err, ErrRefreshTokenInvalid)
	})

	t.Run("unknown token is invalid", func(t *testing.T) {
		_, err := s.RotateRefreshToken(ctx, hashToken("never-stored"), hashToken("new"), future)
		require.ErrorIs(t, err, ErrRefreshTokenInvalid)
	})

	t.Run("expired token is invalid and consumed", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		hE := hashToken("expired")
		_, err := s.StoreRefreshToken(ctx, user.ID, hE, past)
		require.NoError(t, err)

		_, err = s.RotateRefreshToken(ctx, hE, hashToken("new-e"), future)
		require.ErrorIs(t, err, ErrRefreshTokenInvalid)

		// It was deleted on the expired path: a second attempt is still invalid
		// (unknown), not reuse.
		_, err = s.RotateRefreshToken(ctx, hE, hashToken("new-e2"), future)
		require.ErrorIs(t, err, ErrRefreshTokenInvalid)
	})
}
