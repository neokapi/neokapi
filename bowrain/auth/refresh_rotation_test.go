package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
// whole family once the reuse grace window has passed; unknown and expired
// tokens are rejected as invalid.
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

		// Replay the already-consumed hA after the grace window → reuse detected.
		ageConsumption(t, s, hA, RefreshReuseGrace+time.Second)
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

// ageConsumption moves a consumed token's consumed_at, and the created_at of
// the family rows stamped at or after it, back by d. It stands in for a clock:
// the store reads the database clock, so the test moves the rows instead.
func ageConsumption(t *testing.T, s *PostgresAuthStore, tokenHash string, d time.Duration) {
	t.Helper()
	_, err := s.db.Exec(
		`WITH c AS (SELECT family_id, consumed_at FROM refresh_tokens WHERE token_hash = $1)
		 UPDATE refresh_tokens r
		    SET created_at = r.created_at - make_interval(secs => $2)
		   FROM c
		  WHERE r.family_id = c.family_id AND r.created_at >= c.consumed_at`,
		tokenHash, d.Seconds())
	require.NoError(t, err)
	_, err = s.db.Exec(
		`UPDATE refresh_tokens SET consumed_at = consumed_at - make_interval(secs => $2)
		  WHERE token_hash = $1`,
		tokenHash, d.Seconds())
	require.NoError(t, err)
}

// TestRotateRefreshTokenReuseGrace covers the reuse grace window: a consumed
// token presented again within RefreshReuseGrace gets a sibling successor and
// leaves the family intact, up to RefreshReuseSiblingCap; outside the window,
// or past the cap, the presentation is reuse and revokes the family; an expired
// token stays invalid within the window.
func TestRotateRefreshTokenReuseGrace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := &platauth.User{ID: "u-grace-1", Email: "grace@example.com", Name: "Grace"}
	require.NoError(t, s.CreateUser(ctx, user))

	future := time.Now().Add(30 * 24 * time.Hour)

	t.Run("reuse within grace yields a sibling and keeps the first successor", func(t *testing.T) {
		h0 := hashToken("g1-0")
		_, err := s.StoreRefreshToken(ctx, user.ID, h0, future)
		require.NoError(t, err)

		first := hashToken("g1-first")
		_, err = s.RotateRefreshToken(ctx, h0, first, future)
		require.NoError(t, err)

		sibling := hashToken("g1-sibling")
		uid, err := s.RotateRefreshToken(ctx, h0, sibling, future)
		require.NoError(t, err)
		assert.Equal(t, user.ID, uid)

		// Both successors are live and rotate independently.
		uid, err = s.RotateRefreshToken(ctx, first, hashToken("g1-first-next"), future)
		require.NoError(t, err)
		assert.Equal(t, user.ID, uid)
		uid, err = s.RotateRefreshToken(ctx, sibling, hashToken("g1-sibling-next"), future)
		require.NoError(t, err)
		assert.Equal(t, user.ID, uid)
	})

	t.Run("reuse after the grace window revokes the family", func(t *testing.T) {
		h0 := hashToken("g2-0")
		_, err := s.StoreRefreshToken(ctx, user.ID, h0, future)
		require.NoError(t, err)

		first := hashToken("g2-first")
		_, err = s.RotateRefreshToken(ctx, h0, first, future)
		require.NoError(t, err)

		ageConsumption(t, s, h0, RefreshReuseGrace+time.Second)

		_, err = s.RotateRefreshToken(ctx, h0, hashToken("g2-late"), future)
		require.ErrorIs(t, err, ErrRefreshTokenReuse)

		_, err = s.RotateRefreshToken(ctx, first, hashToken("g2-first-next"), future)
		require.ErrorIs(t, err, ErrRefreshTokenInvalid)
	})

	t.Run("the sibling cap revokes the family", func(t *testing.T) {
		h0 := hashToken("g3-0")
		_, err := s.StoreRefreshToken(ctx, user.ID, h0, future)
		require.NoError(t, err)

		successors := []string{hashToken("g3-s0")}
		_, err = s.RotateRefreshToken(ctx, h0, successors[0], future)
		require.NoError(t, err)

		for i := 1; i < RefreshReuseSiblingCap; i++ {
			h := hashToken("g3-s" + string(rune('0'+i)))
			_, err = s.RotateRefreshToken(ctx, h0, h, future)
			require.NoError(t, err, "sibling %d within the cap", i)
			successors = append(successors, h)
		}

		_, err = s.RotateRefreshToken(ctx, h0, hashToken("g3-over"), future)
		require.ErrorIs(t, err, ErrRefreshTokenReuse)

		for _, h := range successors {
			_, err = s.RotateRefreshToken(ctx, h, hashToken(h+"-next"), future)
			require.ErrorIs(t, err, ErrRefreshTokenInvalid)
		}
	})

	t.Run("an expired token within grace is still invalid", func(t *testing.T) {
		h0 := hashToken("g4-0")
		_, err := s.StoreRefreshToken(ctx, user.ID, h0, future)
		require.NoError(t, err)

		first := hashToken("g4-first")
		_, err = s.RotateRefreshToken(ctx, h0, first, future)
		require.NoError(t, err)

		_, err = s.db.Exec(
			`UPDATE refresh_tokens SET expires_at = NOW() - INTERVAL '1 hour' WHERE token_hash = $1`, h0)
		require.NoError(t, err)

		_, err = s.RotateRefreshToken(ctx, h0, hashToken("g4-sibling"), future)
		require.ErrorIs(t, err, ErrRefreshTokenInvalid)

		// The family is intact: the first successor still rotates.
		_, err = s.RotateRefreshToken(ctx, first, hashToken("g4-first-next"), future)
		require.NoError(t, err)
	})

	t.Run("concurrent presentations of one token both succeed", func(t *testing.T) {
		h0 := hashToken("g5-0")
		_, err := s.StoreRefreshToken(ctx, user.ID, h0, future)
		require.NoError(t, err)

		const n = 2
		errs := make(chan error, n)
		for i := range n {
			go func() {
				_, err := s.RotateRefreshToken(ctx, h0, hashToken("g5-s"+string(rune('0'+i))), future)
				errs <- err
			}()
		}
		for range n {
			require.NoError(t, <-errs)
		}
		for i := range n {
			_, err = s.RotateRefreshToken(ctx, hashToken("g5-s"+string(rune('0'+i))), hashToken("g5-next"+string(rune('0'+i))), future)
			require.NoError(t, err)
		}
	})

	// The row lock serializes concurrent presentations, so their outcomes are
	// fixed by position, whichever goroutine wins each round. Each successor
	// is stamped with the database clock read after the lock is held, so the
	// cap count sees every successor committed before it.
	concurrentPresentations := func(t *testing.T, prefix string, consumeFirst bool) (succeeded, reused, invalid int) {
		t.Helper()
		h0 := hashToken(prefix + "-0")
		_, err := s.StoreRefreshToken(ctx, user.ID, h0, future)
		require.NoError(t, err)
		if consumeFirst {
			_, err = s.RotateRefreshToken(ctx, h0, hashToken(prefix+"-first"), future)
			require.NoError(t, err)
		}

		const n = RefreshReuseSiblingCap + 1
		errs := make(chan error, n)
		start := make(chan struct{})
		for i := range n {
			go func() {
				<-start
				_, err := s.RotateRefreshToken(ctx, h0, hashToken(prefix+"-c"+string(rune('0'+i))), future)
				errs <- err
			}()
		}
		close(start)
		for range n {
			err := <-errs
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, ErrRefreshTokenReuse):
				reused++
			case errors.Is(err, ErrRefreshTokenInvalid):
				invalid++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		return succeeded, reused, invalid
	}

	t.Run("concurrent presentations of a fresh token stop at the cap", func(t *testing.T) {
		// The first presentation consumes the token inside the burst, so the
		// cap count depends on the siblings' created_at falling at or after
		// that consumed_at: cap rotations succeed, the next one is reuse.
		succeeded, reused, invalid := concurrentPresentations(t, "g6", false)
		assert.Equal(t, RefreshReuseSiblingCap, succeeded)
		assert.Equal(t, 1, reused)
		assert.Equal(t, 0, invalid)
	})

	t.Run("concurrent presentations of a consumed token stop at the cap", func(t *testing.T) {
		// The first successor already counts toward the cap, so cap-1
		// siblings succeed and the next presentation is reuse. Reuse deletes
		// the family, so a presentation still waiting on the lock finds no
		// row and is invalid.
		succeeded, reused, invalid := concurrentPresentations(t, "g7", true)
		assert.Equal(t, RefreshReuseSiblingCap-1, succeeded)
		assert.Equal(t, 1, reused)
		assert.Equal(t, 1, invalid)
	})
}
