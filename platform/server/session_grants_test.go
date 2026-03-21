package server

import (
	"context"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModePermissionCeiling(t *testing.T) {
	tests := []struct {
		name string
		mode platauth.AgentMode
		want platauth.Permission
	}{
		{"ask mode is read-only", platauth.AgentModeAsk, platauth.PermViewContent},
		{"coworker mode is full access", platauth.AgentModeCoworker, platauth.PermAll},
		{"voice mode is content+brand+review", platauth.AgentModeVoice, platauth.PermViewContent | platauth.PermManageBrand | platauth.PermReview},
		{"unknown mode defaults to view only", platauth.AgentMode("unknown"), platauth.PermViewContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := platauth.ModePermissionCeiling(tt.mode)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreateSessionGrantForMode(t *testing.T) {
	t.Run("ask mode restricts to view only", func(t *testing.T) {
		userPerms := platauth.PermViewContent | platauth.PermTranslate | platauth.PermManageFiles
		grant := CreateSessionGrantForMode("sess-1", "user-1", platauth.AgentModeAsk, userPerms, nil)

		assert.Equal(t, platauth.PermViewContent, grant.Permissions)
		assert.Equal(t, "sess-1", grant.SessionID)
		assert.Equal(t, "user-1", grant.UserID)
		assert.Equal(t, platauth.AgentModeAsk, grant.Mode)
	})

	t.Run("coworker mode preserves all user permissions", func(t *testing.T) {
		userPerms := platauth.PermViewContent | platauth.PermTranslate | platauth.PermReview
		grant := CreateSessionGrantForMode("sess-2", "user-2", platauth.AgentModeCoworker, userPerms, []string{"fr", "de"})

		assert.Equal(t, userPerms, grant.Permissions)
		assert.Equal(t, []string{"fr", "de"}, grant.Languages)
	})

	t.Run("voice mode intersects with user permissions", func(t *testing.T) {
		// User has translate but NOT manage_brand — voice ceiling includes manage_brand
		// but intersection should exclude it since user doesn't have it.
		userPerms := platauth.PermViewContent | platauth.PermTranslate
		grant := CreateSessionGrantForMode("sess-3", "user-3", platauth.AgentModeVoice, userPerms, nil)

		// Voice ceiling: view_content | manage_brand | review
		// User: view_content | translate
		// Intersection: view_content
		assert.Equal(t, platauth.PermViewContent, grant.Permissions)
	})

	t.Run("voice mode with full user permissions", func(t *testing.T) {
		grant := CreateSessionGrantForMode("sess-4", "user-4", platauth.AgentModeVoice, platauth.PermAll, nil)

		expected := platauth.PermViewContent | platauth.PermManageBrand | platauth.PermReview
		assert.Equal(t, expected, grant.Permissions)
	})
}

func TestSessionGrantRoundtrip(t *testing.T) {
	store := NewMemorySessionStore()
	defer store.Close()
	ctx := context.Background()

	grant := &platauth.SessionGrant{
		SessionID:   "conv-abc",
		UserID:      "user-42",
		Permissions: platauth.PermViewContent | platauth.PermTranslate,
		Languages:   []string{"fr", "de"},
		ProjectIDs:  []string{"proj-1"},
		Mode:        platauth.AgentModeCoworker,
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}

	// Set
	err := SetSessionGrant(ctx, store, grant)
	require.NoError(t, err)

	// Get
	got, err := GetSessionGrant(ctx, store, "conv-abc")
	require.NoError(t, err)
	assert.Equal(t, grant.SessionID, got.SessionID)
	assert.Equal(t, grant.UserID, got.UserID)
	assert.Equal(t, grant.Permissions, got.Permissions)
	assert.Equal(t, grant.Languages, got.Languages)
	assert.Equal(t, grant.ProjectIDs, got.ProjectIDs)
	assert.Equal(t, grant.Mode, got.Mode)

	// Delete
	err = DeleteSessionGrant(ctx, store, "conv-abc")
	require.NoError(t, err)

	// Get after delete
	_, err = GetSessionGrant(ctx, store, "conv-abc")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSessionGrantExpiry(t *testing.T) {
	store := NewMemorySessionStore()
	defer store.Close()
	ctx := context.Background()

	// Grant with past ExpiresAt should use defaultGrantTTL instead
	grant := &platauth.SessionGrant{
		SessionID:   "expired-sess",
		UserID:      "user-99",
		Permissions: platauth.PermViewContent,
		Mode:        platauth.AgentModeAsk,
		ExpiresAt:   time.Now().Add(-1 * time.Hour), // already expired
	}

	err := SetSessionGrant(ctx, store, grant)
	require.NoError(t, err)

	// Should still be retrievable because SetSessionGrant uses defaultGrantTTL
	// when ExpiresAt is in the past
	got, err := GetSessionGrant(ctx, store, "expired-sess")
	require.NoError(t, err)
	assert.Equal(t, grant.SessionID, got.SessionID)
}
