package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateToken(t *testing.T) {
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
}

func TestValidateTokenWrongSecret(t *testing.T) {
	user := &User{ID: "user-1", Email: "test@example.com", Name: "Test"}

	token, err := GenerateToken(user, "secret-a", 1*time.Hour)
	require.NoError(t, err)

	_, err = ValidateToken(token, "secret-b")
	assert.Error(t, err)
}

func TestValidateTokenExpired(t *testing.T) {
	user := &User{ID: "user-1", Email: "test@example.com", Name: "Test"}

	token, err := GenerateToken(user, "secret", -1*time.Hour)
	require.NoError(t, err)

	_, err = ValidateToken(token, "secret")
	assert.Error(t, err)
}
