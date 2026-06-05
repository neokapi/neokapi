package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentTokenCreateAndValidate(t *testing.T) {
	store := NewAgentTokenStore()

	token, err := store.Create("user-1", "ws-1", "conv-1", "member", time.Hour)
	require.NoError(t, err)
	assert.Greater(t, len(token.Token), len("bwt_bravo_"))
	assert.Equal(t, "bwt_bravo_", token.Token[:10])
	assert.Equal(t, "user-1", token.UserID)
	assert.Equal(t, "ws-1", token.WorkspaceID)
	assert.Equal(t, "conv-1", token.ConversationID)
	assert.Equal(t, "member", token.WorkspaceRole)

	// Validate.
	got, err := store.Validate(token.Token)
	require.NoError(t, err)
	assert.Equal(t, token.UserID, got.UserID)
}

func TestAgentTokenValidateInvalid(t *testing.T) {
	store := NewAgentTokenStore()
	_, err := store.Validate("bwt_bravo_nonexistent")
	assert.Error(t, err)
}

func TestAgentTokenValidateExpired(t *testing.T) {
	store := NewAgentTokenStore()

	token, err := store.Create("user-1", "ws-1", "conv-1", "member", -time.Second)
	require.NoError(t, err)

	_, err = store.Validate(token.Token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestAgentTokenRevoke(t *testing.T) {
	store := NewAgentTokenStore()

	token, err := store.Create("user-1", "ws-1", "conv-1", "member", time.Hour)
	require.NoError(t, err)

	store.Revoke(token.Token)

	_, err = store.Validate(token.Token)
	assert.Error(t, err)
}

func TestAgentTokenRevokeForConversation(t *testing.T) {
	store := NewAgentTokenStore()

	t1, _ := store.Create("user-1", "ws-1", "conv-1", "member", time.Hour)
	t2, _ := store.Create("user-1", "ws-1", "conv-1", "member", time.Hour)
	t3, _ := store.Create("user-1", "ws-1", "conv-2", "member", time.Hour)

	store.RevokeForConversation("conv-1")

	_, err := store.Validate(t1.Token)
	require.Error(t, err)
	_, err = store.Validate(t2.Token)
	require.Error(t, err)

	// conv-2 token should still be valid.
	_, err = store.Validate(t3.Token)
	assert.NoError(t, err)
}

func TestAgentTokenPurgeExpired(t *testing.T) {
	store := NewAgentTokenStore()

	_, err := store.Create("user-1", "ws-1", "conv-1", "member", -time.Second)
	require.NoError(t, err)
	_, err = store.Create("user-1", "ws-1", "conv-2", "member", -time.Second)
	require.NoError(t, err)
	_, err = store.Create("user-1", "ws-1", "conv-3", "member", time.Hour) // not expired
	require.NoError(t, err)

	count := store.PurgeExpired()
	assert.Equal(t, 2, count)
}
