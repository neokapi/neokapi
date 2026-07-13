package server

import (
	"context"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// planSyncer builds the workspace plan-cache syncer. It lives in the auth package
// because the worker's trial sweeper syncs plans too, and the two must not drift.
func (s *Server) planSyncer() *auth.PlanSyncer {
	return auth.NewPlanSyncer(s.AuthStore)
}

// ownerEmailResolver resolves the workspace owner's email for billing notifications.
type ownerEmailResolver struct {
	authStore auth.AuthStore
}

func (r *ownerEmailResolver) GetOwnerEmail(ctx context.Context, workspaceID string) string {
	members, err := r.authStore.ListMembers(ctx, workspaceID)
	if err != nil {
		return ""
	}
	for _, m := range members {
		if m.Role == platauth.RoleOwner {
			u, err := r.authStore.GetUser(ctx, m.UserID)
			if err == nil {
				return u.Email
			}
		}
	}
	return ""
}

// billingGuardEvent returns a GuardEventFunc that fires PostHog events.
// Returns nil (no-op) when PostHog is not configured.
func (s *Server) billingGuardEvent() billing.GuardEventFunc {
	if s.PostHogClient == nil {
		return nil
	}
	return func(event string, workspaceID string, props map[string]any) {
		s.PostHogClient.CaptureEvent(workspaceID, event, props)
	}
}
