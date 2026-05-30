package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

const sessionGrantPrefix = "grant:"
const defaultGrantTTL = 2 * time.Hour

// SetSessionGrant stores a session grant in the state store.
func SetSessionGrant(ctx context.Context, store SessionStateStore, grant *platauth.SessionGrant) error {
	data, err := json.Marshal(grant)
	if err != nil {
		return fmt.Errorf("marshal session grant: %w", err)
	}
	ttl := time.Until(grant.ExpiresAt)
	if ttl <= 0 {
		ttl = defaultGrantTTL
	}
	return store.Set(ctx, sessionGrantPrefix+grant.SessionID, data, ttl)
}

// GetSessionGrant retrieves a session grant from the state store.
func GetSessionGrant(ctx context.Context, store SessionStateStore, sessionID string) (*platauth.SessionGrant, error) {
	data, err := store.Get(ctx, sessionGrantPrefix+sessionID)
	if err != nil {
		return nil, err
	}
	var grant platauth.SessionGrant
	if err := json.Unmarshal(data, &grant); err != nil {
		return nil, fmt.Errorf("unmarshal session grant: %w", err)
	}
	return &grant, nil
}

// DeleteSessionGrant removes a session grant from the state store.
func DeleteSessionGrant(ctx context.Context, store SessionStateStore, sessionID string) error {
	return store.Delete(ctx, sessionGrantPrefix+sessionID)
}

// CreateSessionGrantForMode creates a session grant for a @bravo conversation
// with permissions restricted to the mode's ceiling intersected with the user's
// base permissions. An optional projectIDs list scopes the grant to specific
// projects; when non-empty, SessionGrantMiddleware denies requests targeting any
// project outside the set.
func CreateSessionGrantForMode(sessionID, userID string, mode platauth.AgentMode, userPermissions platauth.Permission, userLanguages []string, projectIDs ...string) *platauth.SessionGrant {
	ceiling := platauth.ModePermissionCeiling(mode)
	return &platauth.SessionGrant{
		SessionID:   sessionID,
		UserID:      userID,
		Permissions: userPermissions & ceiling, // intersect
		Languages:   userLanguages,
		ProjectIDs:  projectIDs,
		Mode:        mode,
		ExpiresAt:   time.Now().Add(defaultGrantTTL),
	}
}
