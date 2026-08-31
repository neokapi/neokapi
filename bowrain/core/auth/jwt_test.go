package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateToken(t *testing.T) {
	t.Parallel()

	user := &User{ID: "user-1", Email: "test@example.com", Name: "Test User"}
	secret := "test-secret-key-32-bytes-long!!!"

	token, err := GenerateToken(user, secret, 1*time.Hour)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := ValidateToken(token, secret)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.Equal(t, "Test User", claims.Name)
	assert.Equal(t, TokenIssuer, claims.Issuer)
	assert.Contains(t, []string(claims.Audience), TokenAudience)
}

// TestValidateTokenRejectsWrongClaims verifies that ValidateToken enforces the
// Bowrain issuer + audience and requires an expiry, so a token minted for
// another purpose with the same secret cannot be replayed as a session token.
func TestValidateTokenRejectsWrongClaims(t *testing.T) {
	t.Parallel()

	secret := "test-secret-key-32-bytes-long!!!"
	sign := func(c Claims) string {
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
		require.NoError(t, err)
		return s
	}
	now := time.Now()
	base := func() Claims {
		return Claims{
			Issuer:    TokenIssuer,
			Subject:   "u1",
			Audience:  jwt.ClaimStrings{TokenAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}
	}

	// Sanity: a well-formed token validates.
	_, err := ValidateToken(sign(base()), secret)
	require.NoError(t, err)

	t.Run("wrong issuer", func(t *testing.T) {
		c := base()
		c.Issuer = "evil-issuer"
		_, err := ValidateToken(sign(c), secret)
		assert.Error(t, err)
	})
	t.Run("wrong audience", func(t *testing.T) {
		c := base()
		c.Audience = jwt.ClaimStrings{"someone-else"}
		_, err := ValidateToken(sign(c), secret)
		assert.Error(t, err)
	})
	t.Run("missing expiry", func(t *testing.T) {
		c := base()
		c.ExpiresAt = nil
		_, err := ValidateToken(sign(c), secret)
		assert.Error(t, err)
	})
}

// TestAdminTokenAudienceSeparation verifies that admin and user session tokens
// cannot be used interchangeably: each validates only under its own audience.
func TestAdminTokenAudienceSeparation(t *testing.T) {
	t.Parallel()

	secret := "test-secret-key-32-bytes-long!!!"
	user := &User{ID: "u-1", Email: "u@example.com", Name: "User"}

	userTok, err := GenerateToken(user, secret, time.Hour)
	require.NoError(t, err)
	adminTok, err := GenerateAdminToken("admin-sub", "admin@example.com", "Admin", secret, time.Hour)
	require.NoError(t, err)

	// Each validates under its own audience.
	uc, err := ValidateToken(userTok, secret)
	require.NoError(t, err)
	assert.Equal(t, "u-1", uc.Subject)
	ac, err := ValidateAdminToken(adminTok, secret)
	require.NoError(t, err)
	assert.Equal(t, "admin-sub", ac.Subject)
	assert.Equal(t, "admin@example.com", ac.Email)

	// Cross-use is rejected: a user token is not an admin token and vice versa.
	_, err = ValidateAdminToken(userTok, secret)
	require.Error(t, err, "user session token must not validate as admin")
	_, err = ValidateToken(adminTok, secret)
	require.Error(t, err, "admin session token must not validate as user")
}

func TestValidateTokenWrongSecret(t *testing.T) {
	t.Parallel()

	user := &User{ID: "user-1", Email: "test@example.com", Name: "Test"}

	token, err := GenerateToken(user, "secret-a", 1*time.Hour)
	require.NoError(t, err)

	_, err = ValidateToken(token, "secret-b")
	assert.Error(t, err)
}

func TestValidateTokenExpired(t *testing.T) {
	t.Parallel()

	user := &User{ID: "user-1", Email: "test@example.com", Name: "Test"}

	token, err := GenerateToken(user, "secret", -1*time.Hour)
	require.NoError(t, err)

	_, err = ValidateToken(token, "secret")
	assert.Error(t, err)
}

// Setup state travels through another party's URL, so the audience split is
// what keeps it from being useful anywhere else: it can never be presented as
// a session, and no session can be spent as state.
func TestSetupState(t *testing.T) {
	t.Parallel()

	const secret = "test-secret-key-32-bytes-long!!!"

	state, err := GenerateSetupState("ws-1", secret, time.Hour)
	require.NoError(t, err)

	wsID, err := ValidateSetupState(state, secret)
	require.NoError(t, err)
	assert.Equal(t, "ws-1", wsID)

	t.Run("a session token is not setup state", func(t *testing.T) {
		t.Parallel()
		session, err := GenerateToken(&User{ID: "u1", Email: "u@example.com"}, secret, time.Hour)
		require.NoError(t, err)
		_, err = ValidateSetupState(session, secret)
		require.Error(t, err)
	})

	t.Run("setup state is not a session token", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateToken(state, secret)
		require.Error(t, err)
		_, err = ValidateAdminToken(state, secret)
		require.Error(t, err)
	})

	t.Run("expired state is refused", func(t *testing.T) {
		t.Parallel()
		expired, err := GenerateSetupState("ws-1", secret, -time.Minute)
		require.NoError(t, err)
		_, err = ValidateSetupState(expired, secret)
		require.Error(t, err)
	})

	t.Run("another key does not verify", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateSetupState(state, "a-different-secret-key-32-bytes!")
		require.Error(t, err)
	})

	t.Run("fails closed without a workspace or a secret", func(t *testing.T) {
		t.Parallel()
		_, err := GenerateSetupState("", secret, time.Hour)
		require.Error(t, err, "state that names no workspace grants nothing and must not be minted")
		_, err = GenerateSetupState("ws-1", "", time.Hour)
		require.ErrorIs(t, err, ErrEmptySecret)
		_, err = ValidateSetupState(state, "")
		require.ErrorIs(t, err, ErrEmptySecret)
	})
}

func TestGenerateRefreshToken(t *testing.T) {
	t.Parallel()

	token1, err := GenerateRefreshToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token1)
	// base64url encoded 32 bytes = 44 chars (with padding).
	assert.Len(t, token1, 44)

	token2, err := GenerateRefreshToken()
	require.NoError(t, err)
	assert.NotEqual(t, token1, token2, "refresh tokens should be unique")
}
