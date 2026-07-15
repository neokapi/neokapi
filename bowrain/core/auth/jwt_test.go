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
		return Claims{RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    TokenIssuer,
			Subject:   "u1",
			Audience:  jwt.ClaimStrings{TokenAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		}}
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
